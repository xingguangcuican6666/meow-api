package service

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResetStatusCode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		statusCode       int
		statusCodeConfig string
		expectedCode     int
	}{
		{
			name:             "map string value",
			statusCode:       429,
			statusCodeConfig: `{"429":"503"}`,
			expectedCode:     503,
		},
		{
			name:             "map int value",
			statusCode:       429,
			statusCodeConfig: `{"429":503}`,
			expectedCode:     503,
		},
		{
			name:             "skip invalid string value",
			statusCode:       429,
			statusCodeConfig: `{"429":"bad-code"}`,
			expectedCode:     429,
		},
		{
			name:             "skip status code 200",
			statusCode:       200,
			statusCodeConfig: `{"200":503}`,
			expectedCode:     200,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			newAPIError := &types.NewAPIError{
				StatusCode: tc.statusCode,
			}
			ResetStatusCode(newAPIError, tc.statusCodeConfig)
			require.Equal(t, tc.expectedCode, newAPIError.StatusCode)
		})
	}
}

func TestRelayErrorHandlerTruncatesInvalidJSONBodyInLog(t *testing.T) {
	withDebugEnabled(t, false)

	body := strings.Repeat("b", common.LocalLogContentLimit+256)
	var logBuffer bytes.Buffer

	common.LogWriterMu.Lock()
	oldWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &logBuffer
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultErrorWriter = oldWriter
		common.LogWriterMu.Unlock()
	})

	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	newAPIError := RelayErrorHandler(newErrorTestContext(t), resp, false)

	require.NotNil(t, newAPIError)
	require.Equal(t, "bad response status code 500", newAPIError.Error())
	require.Contains(t, logBuffer.String(), "[truncated")
	require.Contains(t, logBuffer.String(), fmt.Sprintf("original_length=%d", len(body)))
	require.NotContains(t, logBuffer.String(), strings.Repeat("b", common.LocalLogContentLimit+1))
}

func TestRelayErrorHandlerKeepsStructuredErrorMessage(t *testing.T) {
	message := strings.Repeat("c", common.LocalLogContentLimit+256)
	body := `{"message":"` + message + `"}`
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	newAPIError := RelayErrorHandler(newErrorTestContext(t), resp, false)

	require.NotNil(t, newAPIError)
	require.Equal(t, message, newAPIError.Error())
}

func TestRelayErrorHandlerKeepsOpenAIErrorMessage(t *testing.T) {
	message := strings.Repeat("d", common.LocalLogContentLimit+256)
	body := `{"error":{"message":"` + message + `","type":"server_error","code":"server_error"}}`
	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	newAPIError := RelayErrorHandler(newErrorTestContext(t), resp, false)

	require.NotNil(t, newAPIError)
	require.Equal(t, message, newAPIError.Error())
}

func TestRelayErrorHandlerKeepsInvalidJSONBodyInDebugLog(t *testing.T) {
	withDebugEnabled(t, true)

	body := strings.Repeat("e", common.LocalLogContentLimit+256)
	var logBuffer bytes.Buffer

	common.LogWriterMu.Lock()
	oldWriter := gin.DefaultErrorWriter
	gin.DefaultErrorWriter = &logBuffer
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultErrorWriter = oldWriter
		common.LogWriterMu.Unlock()
	})

	resp := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	newAPIError := RelayErrorHandler(newErrorTestContext(t), resp, false)

	require.NotNil(t, newAPIError)
	require.NotContains(t, logBuffer.String(), "[truncated")
	require.Contains(t, logBuffer.String(), body)
}

func withDebugEnabled(t *testing.T, enabled bool) {
	t.Helper()

	oldDebug := common.DebugEnabled
	common.DebugEnabled = enabled
	t.Cleanup(func() {
		common.DebugEnabled = oldDebug
	})
}

func newErrorTestContext(t *testing.T) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return c
}

func TestRelayErrorHandlerSanitizesStructuredError(t *testing.T) {
	original := setting.SanitizeUpstreamErrorEnabled
	setting.SanitizeUpstreamErrorEnabled = true
	t.Cleanup(func() { setting.SanitizeUpstreamErrorEnabled = original })

	upstreamMessage := "account acct-secret has balance 0"
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"` + upstreamMessage + `","type":"rate_limit_error","code":"rate_limit_exceeded"}}`)),
	}
	c := newErrorTestContext(t)
	newAPIError := RelayErrorHandler(c, resp, false)
	// Standardization is applied once at the relay exit (controller.Relay defer),
	// not inside RelayErrorHandler; replicate that exit step here.
	StandardizeUpstreamError(c, newAPIError)

	// The client projection carries the fixed sentence; the internal error
	// keeps the verbatim upstream text for keyword matching and error logs.
	require.Equal(t, StandardUpstreamMessage(c, http.StatusTooManyRequests), newAPIError.ToOpenAIError().Message)
	require.Contains(t, newAPIError.Err.Error(), upstreamMessage)
	require.Equal(t, http.StatusTooManyRequests, newAPIError.StatusCode)
}

func TestRelayErrorHandlerSanitizesClaudeProjection(t *testing.T) {
	original := setting.SanitizeUpstreamErrorEnabled
	setting.SanitizeUpstreamErrorEnabled = true
	t.Cleanup(func() { setting.SanitizeUpstreamErrorEnabled = original })

	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"internal host upstream-a.example.com quota exhausted","type":"rate_limit_error","code":"rate_limit_exceeded"}}`)),
	}
	c := newErrorTestContext(t)
	newAPIError := RelayErrorHandler(c, resp, false)
	StandardizeUpstreamError(c, newAPIError)

	claudeError := newAPIError.ToClaudeError()
	require.Equal(t, StandardUpstreamMessage(c, http.StatusTooManyRequests), claudeError.Message)
	require.Contains(t, newAPIError.Err.Error(), "upstream-a.example.com")
}

// StandardizeUpstreamError must also override a client message that a caller
// pinned by hand: the user-channel breaker replays a verbatim upstream failure
// under the local ErrorCodeGetChannelFailed code, which the allowlist would
// otherwise pass through untouched.
func TestStandardizeUpstreamErrorOverridesPinnedClientMessage(t *testing.T) {
	original := setting.SanitizeUpstreamErrorEnabled
	setting.SanitizeUpstreamErrorEnabled = true
	t.Cleanup(func() { setting.SanitizeUpstreamErrorEnabled = original })

	upstreamMessage := "account acct-secret has balance 0"
	c := newErrorTestContext(t)
	echoed := types.NewError(
		errors.New(upstreamMessage),
		types.ErrorCodeGetChannelFailed,
		types.ErrOptionWithStatusCode(http.StatusBadGateway),
		types.ErrOptionWithSkipRetry(),
	)
	// The breaker replay path pins the verbatim text before the relay exit runs.
	echoed.SetClientMessage(StandardUpstreamMessage(c, http.StatusBadGateway))

	StandardizeUpstreamError(c, echoed)

	require.Equal(t, StandardUpstreamMessage(c, http.StatusBadGateway), echoed.ToOpenAIError().Message)
	require.NotContains(t, echoed.ToOpenAIError().Message, "acct-secret")
	// Err keeps the verbatim upstream text for the request's error log.
	require.Contains(t, echoed.Err.Error(), upstreamMessage)
}

func TestRelayErrorHandlerKeepsVerbatimErrorForAdminAndRoot(t *testing.T) {
	original := setting.SanitizeUpstreamErrorEnabled
	setting.SanitizeUpstreamErrorEnabled = true
	t.Cleanup(func() { setting.SanitizeUpstreamErrorEnabled = original })

	upstreamMessage := "account acct-secret has balance 0"
	for name, role := range map[string]int{
		"admin": common.RoleAdminUser,
		"root":  common.RoleRootUser,
	} {
		t.Run(name, func(t *testing.T) {
			// Privileged callers keep seeing the verbatim upstream error, so admin
			// detection must run through the real model.IsAdmin lookup keyed by the
			// context user id rather than a bare role flag.
			user := &model.User{
				Username: "sanitize-verbatim-" + name,
				Role:     role,
				Status:   common.UserStatusEnabled,
				AffCode:  "sanitize-verbatim-" + name,
			}
			require.NoError(t, model.DB.Create(user).Error)
			t.Cleanup(func() { model.DB.Unscoped().Delete(user) })

			resp := &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"` + upstreamMessage + `","type":"rate_limit_error","code":"rate_limit_exceeded"}}`)),
			}
			c := newErrorTestContext(t)
			c.Set("id", user.Id)
			newAPIError := RelayErrorHandler(c, resp, false)
			StandardizeUpstreamError(c, newAPIError)
			require.Contains(t, newAPIError.ToOpenAIError().Message, upstreamMessage)
			require.Empty(t, newAPIError.GetClientMessage())
		})
	}
}

func TestShouldRuntimeDisableMatchesVerbatimUpstreamKeywordWhileSanitized(t *testing.T) {
	original := setting.SanitizeUpstreamErrorEnabled
	setting.SanitizeUpstreamErrorEnabled = true
	t.Cleanup(func() { setting.SanitizeUpstreamErrorEnabled = original })
	originalRuntimeDisable := common.RuntimeAutomaticDisableChannelEnabled
	common.RuntimeAutomaticDisableChannelEnabled = true
	t.Cleanup(func() { common.RuntimeAutomaticDisableChannelEnabled = originalRuntimeDisable })

	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"account acct-secret has balance 0","type":"insufficient_quota","code":"insufficient_quota"}}`)),
	}
	c := newErrorTestContext(t)
	newAPIError := RelayErrorHandler(c, resp, false)
	StandardizeUpstreamError(c, newAPIError)
	require.Equal(t, StandardUpstreamMessage(c, http.StatusTooManyRequests), newAPIError.ToOpenAIError().Message)

	// The auto-disable keyword rule must compare against the verbatim upstream
	// error, not the sanitized client text.
	settings := &dto.ChannelOtherSettings{
		RuntimeAutomaticDisableOverrideEnabled: true,
		RuntimeAutomaticDisableKeywords:        "balance 0",
	}
	require.True(t, ShouldRuntimeDisableChannel(newAPIError, settings))

	settings.RuntimeAutomaticDisableKeywords = "no-match-keyword"
	require.False(t, ShouldRuntimeDisableChannel(newAPIError, settings))
}

func TestRelayErrorHandlerKeepsStructuredErrorWhenSanitizerDisabled(t *testing.T) {
	original := setting.SanitizeUpstreamErrorEnabled
	setting.SanitizeUpstreamErrorEnabled = false
	t.Cleanup(func() { setting.SanitizeUpstreamErrorEnabled = original })

	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"actionable upstream detail","type":"invalid_request_error","code":"invalid_request"}}`)),
	}
	newAPIError := RelayErrorHandler(newErrorTestContext(t), resp, false)
	require.Contains(t, newAPIError.Err.Error(), "actionable upstream detail")
}

func TestStandardizeUpstreamErrorTrustsOnlyLocalProvenance(t *testing.T) {
	original := setting.SanitizeUpstreamErrorEnabled
	setting.SanitizeUpstreamErrorEnabled = true
	t.Cleanup(func() { setting.SanitizeUpstreamErrorEnabled = original })

	tests := []struct {
		name          string
		err           *types.NewAPIError
		secret        string
		wantSanitized bool
	}{
		{
			name:   "local error with an allowlisted code keeps its message",
			err:    types.NewErrorWithStatusCode(errors.New("quota insufficient for user 123"), types.ErrorCodeInsufficientUserQuota, http.StatusPaymentRequired),
			secret: "user 123",
		},
		{
			name:          "local error with a code outside the allowlist is sanitized",
			err:           types.NewOpenAIError(errors.New("upstream.internal.example refused the connection"), types.ErrorCodeDoRequestFailed, http.StatusBadGateway),
			secret:        "upstream.internal.example",
			wantSanitized: true,
		},
		{
			name:          "upstream error reusing an allowlisted code is sanitized",
			err:           types.WithOpenAIError(types.OpenAIError{Message: "account sk-SECRET balance 0", Code: "invalid_request"}, http.StatusUnauthorized),
			secret:        "sk-SECRET",
			wantSanitized: true,
		},
		{
			name:          "upstream error reusing a channel-prefixed code is sanitized",
			err:           types.WithOpenAIError(types.OpenAIError{Message: "account sk-SECRET balance 0", Code: string(types.ErrorCodeChannelInvalidKey)}, http.StatusForbidden),
			secret:        "sk-SECRET",
			wantSanitized: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newErrorTestContext(t)

			StandardizeUpstreamError(c, tt.err)

			assert.Contains(t, tt.err.Error(), tt.secret, "Err keeps the verbatim text for admin logs")
			if !tt.wantSanitized {
				assert.Empty(t, tt.err.GetClientMessage())
				return
			}
			assert.Equal(t, StandardUpstreamMessage(c, tt.err.StatusCode), tt.err.GetClientMessage())
			assert.NotContains(t, tt.err.GetClientMessage(), tt.secret)
		})
	}
}

func TestStandardizeUpstreamTaskErrorTrustsOnlyLocalErrors(t *testing.T) {
	original := setting.SanitizeUpstreamErrorEnabled
	setting.SanitizeUpstreamErrorEnabled = true
	t.Cleanup(func() { setting.SanitizeUpstreamErrorEnabled = original })

	tests := []struct {
		name          string
		taskErr       *taskdto.TaskError
		wantSanitized bool
	}{
		{
			name:    "author-marked local error keeps its message",
			taskErr: TaskErrorWrapperLocal(errors.New("account acct-secret balance 0"), "get_channel_failed", http.StatusServiceUnavailable),
		},
		{
			name:          "unmarked error reusing an allowlisted code is sanitized",
			taskErr:       TaskErrorWrapper(errors.New("account acct-secret balance 0"), "get_channel_failed", http.StatusServiceUnavailable),
			wantSanitized: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newErrorTestContext(t)

			replaced := StandardizeUpstreamTaskError(c, tt.taskErr)

			assert.Equal(t, tt.wantSanitized, replaced)
			if !tt.wantSanitized {
				assert.Contains(t, tt.taskErr.Message, "acct-secret")
				return
			}
			assert.Equal(t, StandardUpstreamMessage(c, http.StatusServiceUnavailable), tt.taskErr.Message)
			assert.NotContains(t, tt.taskErr.Message, "acct-secret")
			assert.Contains(t, tt.taskErr.Error.Error(), "acct-secret", "Error keeps the verbatim text for admin logs")
		})
	}
}
