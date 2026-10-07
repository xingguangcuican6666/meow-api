package zhipu

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// https://open.bigmodel.cn/doc/api#chatglm_std
// chatglm_std, chatglm_lite
// https://open.bigmodel.cn/api/paas/v3/model-api/chatglm_std/invoke
// https://open.bigmodel.cn/api/paas/v3/model-api/chatglm_std/sse-invoke

var zhipuTokens sync.Map
var expSeconds int64 = 24 * 3600

func getZhipuToken(apikey string) string {
	data, ok := zhipuTokens.Load(apikey)
	if ok {
		tokenData := data.(zhipuTokenData)
		if time.Now().Before(tokenData.ExpiryTime) {
			return tokenData.Token
		}
	}

	split := strings.Split(apikey, ".")
	if len(split) != 2 {
		common.SysLog("invalid zhipu key: " + apikey)
		return ""
	}

	id := split[0]
	secret := split[1]

	expMillis := time.Now().Add(time.Duration(expSeconds)*time.Second).UnixNano() / 1e6
	expiryTime := time.Now().Add(time.Duration(expSeconds) * time.Second)

	timestamp := time.Now().UnixNano() / 1e6

	payload := jwt.MapClaims{
		"api_key":   id,
		"exp":       expMillis,
		"timestamp": timestamp,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, payload)

	token.Header["alg"] = "HS256"
	token.Header["sign_type"] = "SIGN"

	tokenString, err := token.SignedString([]byte(secret))
	if err != nil {
		return ""
	}

	zhipuTokens.Store(apikey, zhipuTokenData{
		Token:      tokenString,
		ExpiryTime: expiryTime,
	})

	return tokenString
}

func requestOpenAI2Zhipu(request dto.GeneralOpenAIRequest) *ZhipuRequest {
	messages := make([]ZhipuMessage, 0, len(request.Messages))
	for _, message := range request.Messages {
		if message.Role == "system" {
			messages = append(messages, ZhipuMessage{
				Role:    "system",
				Content: message.StringContent(),
			})
			messages = append(messages, ZhipuMessage{
				Role:    "user",
				Content: "Okay",
			})
		} else {
			messages = append(messages, ZhipuMessage{
				Role:    message.Role,
				Content: message.StringContent(),
			})
		}
	}
	return &ZhipuRequest{
		Prompt:      messages,
		Temperature: request.Temperature,
		TopP:        lo.FromPtrOr(request.TopP, 0),
		Incremental: false,
	}
}

func responseZhipu2OpenAI(response *ZhipuResponse) *dto.OpenAITextResponse {
	fullTextResponse := dto.OpenAITextResponse{
		Id:      response.Data.TaskId,
		Object:  "chat.completion",
		Created: common.GetTimestamp(),
		Choices: make([]dto.OpenAITextResponseChoice, 0, len(response.Data.Choices)),
		Usage:   response.Data.Usage,
	}
	for i, choice := range response.Data.Choices {
		openaiChoice := dto.OpenAITextResponseChoice{
			Index: i,
			Message: dto.Message{
				Role:    choice.Role,
				Content: strings.Trim(choice.Content, "\""),
			},
			FinishReason: "",
		}
		if i == len(response.Data.Choices)-1 {
			openaiChoice.FinishReason = "stop"
		}
		fullTextResponse.Choices = append(fullTextResponse.Choices, openaiChoice)
	}
	return &fullTextResponse
}

func streamResponseZhipu2OpenAI(zhipuResponse string) *dto.ChatCompletionsStreamResponse {
	var choice dto.ChatCompletionsStreamResponseChoice
	choice.Delta.SetContentString(zhipuResponse)
	response := dto.ChatCompletionsStreamResponse{
		Object:  "chat.completion.chunk",
		Created: common.GetTimestamp(),
		Model:   "chatglm",
		Choices: []dto.ChatCompletionsStreamResponseChoice{choice},
	}
	return &response
}

func streamMetaResponseZhipu2OpenAI(zhipuResponse *ZhipuStreamMetaResponse) (*dto.ChatCompletionsStreamResponse, *dto.Usage) {
	var choice dto.ChatCompletionsStreamResponseChoice
	choice.Delta.SetContentString("")
	choice.FinishReason = &constant.FinishReasonStop
	response := dto.ChatCompletionsStreamResponse{
		Id:      zhipuResponse.RequestId,
		Object:  "chat.completion.chunk",
		Created: common.GetTimestamp(),
		Model:   "chatglm",
		Choices: []dto.ChatCompletionsStreamResponseChoice{choice},
	}
	return &response, &zhipuResponse.Usage
}

// zhipuStreamData is one "data:" line of a sse-invoke stream together with the
// type of the SSE event it belongs to: "add" (model output), "error",
// "interrupted" or "finish". Event is empty when no event line precedes it.
type zhipuStreamData struct {
	Event string
	Text  string
}

// zhipuUpstreamErrorFrame is the frame handed to the shared stream sanitizer
// for every upstream failure. It is an OpenAI-style error object that carries
// none of the upstream's text or codes: the sanitizer answers the client with
// its fixed phrasing and the verbatim upstream text goes to the log instead.
const zhipuUpstreamErrorFrame = `{"error":{}}`

// isZhipuStreamFailure reports whether a stream data line is an upstream
// failure rather than model output.
//
// The decision follows the stream protocol instead of guessing from the text.
// "add" events carry model output and are never failures, however much the text
// resembles an error payload. "error" and "interrupted" events are failures
// whatever their text. A line outside those events (no event line, or an
// unknown one) is a failure only when it is a JSON object holding an
// OpenAI-style "error" object or an explicit "success": false.
func isZhipuStreamFailure(event, text string) bool {
	switch event {
	case "add":
		return false
	case "error", "interrupted":
		return true
	}
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "{") {
		return false
	}
	var envelope struct {
		Error   common.RawMessage `json:"error"`
		Success *bool             `json:"success"`
	}
	if common.UnmarshalJsonStr(trimmed, &envelope) != nil {
		return false
	}
	return common.GetJsonType(envelope.Error) == "object" || (envelope.Success != nil && !*envelope.Success)
}

func zhipuStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	var usage *dto.Usage
	var responseText strings.Builder
	scanner := helper.NewStreamScanner(resp.Body)
	scanner.Split(bufio.ScanLines)
	dataChan := make(chan zhipuStreamData)
	metaChan := make(chan string)
	stopChan := make(chan bool)
	go func() {
		event := ""
		for scanner.Scan() {
			data := scanner.Text()
			lines := strings.Split(data, "\n")
			for i, line := range lines {
				if line == "" {
					// A blank line ends the SSE event, and its type with it.
					event = ""
					continue
				}
				if len(line) < 5 {
					continue
				}
				if line[:5] == "data:" {
					dataChan <- zhipuStreamData{Event: event, Text: line[5:]}
					if i != len(lines)-1 {
						dataChan <- zhipuStreamData{Event: event, Text: "\n"}
					}
				} else if line[:5] == "meta:" {
					metaChan <- line[5:]
				} else if strings.HasPrefix(line, "event:") {
					event = strings.TrimSpace(line[6:])
				}
			}
		}
		if err := scanner.Err(); err != nil {
			common.SysLog("error reading stream: " + err.Error())
		}
		stopChan <- true
	}()
	helper.SetEventStreamHeaders(c)
	// After an upstream failure has been answered with the standardized error
	// frame, the remaining frames are still consumed (the reader goroutine blocks
	// on its unbuffered sends) but no longer reach the client. A trailing meta
	// frame still supplies the usage.
	errorSent := false
	c.Stream(func(w io.Writer) bool {
		select {
		case data := <-dataChan:
			if isZhipuStreamFailure(data.Event, data.Text) {
				// The verbatim upstream text is for administrators and stays in the
				// log. The shared sanitizer answers a client it applies to with the
				// standardized error frame; administrators and sanitizer-disabled
				// deployments fall through and keep the verbatim text.
				logger.LogError(c, fmt.Sprintf("zhipu upstream stream error (event: %q): %s", data.Event, common.LocalLogPreview(data.Text)))
				if !errorSent && helper.MaybeWriteSanitizedStreamError(c, info, zhipuUpstreamErrorFrame) {
					errorSent = true
				}
			}
			if errorSent {
				return true
			}
			response := streamResponseZhipu2OpenAI(data.Text)
			jsonResponse, err := json.Marshal(response)
			if err != nil {
				common.SysLog("error marshalling stream response: " + err.Error())
				return true
			}
			responseText.WriteString(data.Text)
			c.Render(-1, common.CustomEvent{Data: "data: " + string(jsonResponse)})
			return true
		case data := <-metaChan:
			var zhipuResponse ZhipuStreamMetaResponse
			err := json.Unmarshal([]byte(data), &zhipuResponse)
			if err != nil {
				common.SysLog("error unmarshalling stream response: " + err.Error())
				return true
			}
			response, zhipuUsage := streamMetaResponseZhipu2OpenAI(&zhipuResponse)
			jsonResponse, err := json.Marshal(response)
			if err != nil {
				common.SysLog("error marshalling stream response: " + err.Error())
				return true
			}
			usage = zhipuUsage
			if errorSent {
				return true
			}
			c.Render(-1, common.CustomEvent{Data: "data: " + string(jsonResponse)})
			return true
		case <-stopChan:
			c.Render(-1, common.CustomEvent{Data: "data: [DONE]"})
			return false
		}
	})
	service.CloseResponseBodyGracefully(resp)
	if usage == nil {
		// No meta frame arrived (an upstream failure or a truncated stream). Settle
		// on an estimate instead of handing the caller a nil usage.
		usage = service.ResponseText2Usage(c, responseText.String(), info.UpstreamModelName, info.GetEstimatePromptTokens())
	}
	return usage, nil
}

func zhipuHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	var zhipuResponse ZhipuResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)
	err = json.Unmarshal(responseBody, &zhipuResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if !zhipuResponse.Success {
		return nil, types.WithOpenAIError(types.OpenAIError{
			Message: zhipuResponse.Msg,
			Code:    zhipuResponse.Code,
		}, resp.StatusCode)
	}
	fullTextResponse := responseZhipu2OpenAI(&zhipuResponse)
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	return &fullTextResponse.Usage, nil
}
