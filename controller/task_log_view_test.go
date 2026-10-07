package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskLogDTOSeparatesUserAdminAndRootDetails(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_public",
		Platform: "document-parser",
		PrivateData: model.TaskPrivateData{
			Key:            "channel-secret-canary",
			UpstreamTaskID: "upstream-private",
			NodeName:       "node-a",
			Execution: &model.TaskExecutionSnapshot{
				RequestID:   "request-public",
				RequestPath: "/v1/documents",
				TaskPlugin: &model.TaskPluginSnapshot{
					Key:     "document-parser",
					Name:    "Document Parser",
					Version: "1.2.3",
					Author: &model.TaskPluginAuthorSnapshot{
						Name: "Community Author",
						URL:  "https://plugins.example/author",
					},
					APIVersion: 1,
					Generation: 42,
				},
			},
		},
	}

	userView := tasksToDto([]*model.Task{task}, false, common.RoleCommonUser)[0]
	assert.Nil(t, userView.AdminInfo)
	assert.Nil(t, userView.RootInfo)

	adminView := tasksToDto([]*model.Task{task}, false, common.RoleAdminUser)[0]
	require.NotNil(t, adminView.AdminInfo)
	require.NotNil(t, adminView.AdminInfo.TaskPlugin)
	assert.Equal(t, "document-parser", adminView.AdminInfo.TaskPlugin.Key)
	assert.Equal(t, "Document Parser", adminView.AdminInfo.TaskPlugin.Name)
	assert.Equal(t, "1.2.3", adminView.AdminInfo.TaskPlugin.Version)
	require.NotNil(t, adminView.AdminInfo.TaskPlugin.Author)
	assert.Equal(t, "Community Author", adminView.AdminInfo.TaskPlugin.Author.Name)
	assert.Equal(t, "https://plugins.example/author", adminView.AdminInfo.TaskPlugin.Author.URL)
	assert.Equal(t, "request-public", adminView.AdminInfo.RequestID)
	assert.Equal(t, "/v1/documents", adminView.AdminInfo.RequestPath)
	assert.Nil(t, adminView.RootInfo)

	rootView := tasksToDto([]*model.Task{task}, false, common.RoleRootUser)[0]
	require.NotNil(t, rootView.AdminInfo)
	require.NotNil(t, rootView.RootInfo)
	require.NotNil(t, rootView.RootInfo.TaskPlugin)
	assert.Equal(t, 1, rootView.RootInfo.TaskPlugin.APIVersion)
	assert.Equal(t, uint64(42), rootView.RootInfo.TaskPlugin.Generation)
	assert.Equal(t, "upstream-private", rootView.RootInfo.UpstreamTaskID)
	assert.Equal(t, "node-a", rootView.RootInfo.NodeName)

	adminJSON, err := common.Marshal(adminView)
	require.NoError(t, err)
	assert.NotContains(t, string(adminJSON), "channel-secret-canary")
	assert.NotContains(t, string(adminJSON), "upstream-private")

	rootJSON, err := common.Marshal(rootView)
	require.NoError(t, err)
	assert.NotContains(t, string(rootJSON), "channel-secret-canary")
	assert.Contains(t, string(rootJSON), "upstream-private")
}

func TestTaskLogDTODoesNotInventHistoricalPluginProvenance(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_without_snapshot",
		Platform: "document-parser",
	}

	adminView := tasksToDto([]*model.Task{task}, false, common.RoleAdminUser)[0]

	assert.Nil(t, adminView.AdminInfo)
	assert.Nil(t, adminView.RootInfo)
}

func TestTaskLogDTOReplacesLegacyVideoURLWithAvailabilityFlag(t *testing.T) {
	task := &model.Task{
		TaskID:     "task_legacy_video",
		Platform:   "jimeng",
		Action:     constant.TaskActionTextToVideo,
		Status:     model.TaskStatusSuccess,
		FailReason: "https://private-upstream.invalid/video.mp4?signature=secret",
	}

	view := tasksToDto([]*model.Task{task}, false, common.RoleCommonUser)[0]
	assert.True(t, view.LegacyVideoAvailable)
	assert.Empty(t, view.ResultURL)
	assert.Empty(t, view.FailReason)
	encoded, err := common.Marshal(view)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "private-upstream.invalid")
	assert.NotContains(t, string(encoded), "result_url")
	assert.Contains(t, string(encoded), "legacy_video_available")
}

func TestTaskLogDTOKeepsFailureReasonAndDoesNotMarkPluginTaskLegacy(t *testing.T) {
	failed := &model.Task{
		TaskID:     "task_failed",
		Platform:   "jimeng",
		Action:     constant.TaskActionTextToVideo,
		Status:     model.TaskStatusFailure,
		FailReason: "provider rejected the request",
	}
	failedView := tasksToDto([]*model.Task{failed}, false, common.RoleCommonUser)[0]
	assert.Equal(t, "provider rejected the request", failedView.FailReason)
	assert.False(t, failedView.LegacyVideoAvailable)

	pluginTask := &model.Task{
		TaskID:     "task_plugin_video",
		Platform:   "community-video",
		Action:     constant.TaskActionTextToVideo,
		Status:     model.TaskStatusSuccess,
		FailReason: "https://stale-upstream.invalid/plugin-video.mp4",
		PrivateData: model.TaskPrivateData{
			ResultURL: "https://private-upstream.invalid/plugin-video.mp4",
			Execution: &model.TaskExecutionSnapshot{
				TaskPlugin: &model.TaskPluginSnapshot{Key: "community-video"},
			},
		},
	}
	pluginView := tasksToDto([]*model.Task{pluginTask}, false, common.RoleCommonUser)[0]
	assert.False(t, pluginView.LegacyVideoAvailable)
	assert.Empty(t, pluginView.ResultURL)
	assert.Empty(t, pluginView.FailReason)
}

func TestTaskLogDTOHidesUpstreamModelMappingUnderPrivacyProtection(t *testing.T) {
	origPrivacy := setting.UpstreamPrivacyProtectionEnabled
	defer func() { setting.UpstreamPrivacyProtectionEnabled = origPrivacy }()

	task := &model.Task{
		TaskID:   "task_model_mapped",
		Platform: "suno",
		Properties: model.Properties{
			OriginModelName:   "public-model",
			UpstreamModelName: "upstream-secret-model",
		},
	}

	setting.UpstreamPrivacyProtectionEnabled = true

	userJSON, err := common.Marshal(tasksToDto([]*model.Task{task}, false, common.RoleCommonUser)[0])
	require.NoError(t, err)
	assert.NotContains(t, string(userJSON), "upstream-secret-model")
	assert.NotContains(t, string(userJSON), "upstream_model_name")
	assert.Contains(t, string(userJSON), "public-model")

	adminJSON, err := common.Marshal(tasksToDto([]*model.Task{task}, false, common.RoleAdminUser)[0])
	require.NoError(t, err)
	assert.Contains(t, string(adminJSON), "upstream-secret-model")

	setting.UpstreamPrivacyProtectionEnabled = false

	openJSON, err := common.Marshal(tasksToDto([]*model.Task{task}, false, common.RoleCommonUser)[0])
	require.NoError(t, err)
	assert.Contains(t, string(openJSON), "upstream-secret-model")
}

func setUpstreamErrorSanitizer(t *testing.T, enabled bool) {
	t.Helper()
	previous := setting.SanitizeUpstreamErrorEnabled
	setting.SanitizeUpstreamErrorEnabled = enabled
	t.Cleanup(func() { setting.SanitizeUpstreamErrorEnabled = previous })
}

const taskFailUpstreamSecret = "account acct-secret-77 on https://api.vendor.example/v1 has balance 0"

func TestTaskLogDTOProjectsUpstreamFailReasonForNonAdmins(t *testing.T) {
	failed := func(reason string) *model.Task {
		return &model.Task{TaskID: "task_failed_secret", Platform: "kling", Status: model.TaskStatusFailure, FailReason: reason}
	}

	setUpstreamErrorSanitizer(t, true)
	userView := tasksToDto([]*model.Task{failed(taskFailUpstreamSecret)}, false, common.RoleCommonUser)[0]
	assert.Equal(t, service.StandardUpstreamMessage(nil, 0), userView.FailReason)
	// result_url falls back to FailReason for legacy rows; it must not echo it.
	assert.Empty(t, userView.ResultURL)
	userJSON, err := common.Marshal(userView)
	require.NoError(t, err)
	assert.NotContains(t, string(userJSON), "acct-secret-77")
	assert.NotContains(t, string(userJSON), "vendor.example")

	for _, role := range []int{common.RoleAdminUser, common.RoleRootUser} {
		adminView := tasksToDto([]*model.Task{failed(taskFailUpstreamSecret)}, false, role)[0]
		assert.Equal(t, taskFailUpstreamSecret, adminView.FailReason)
	}

	localView := tasksToDto([]*model.Task{failed("任务超时（5分钟）")}, false, common.RoleCommonUser)[0]
	assert.Equal(t, "任务超时（5分钟）", localView.FailReason)

	setUpstreamErrorSanitizer(t, false)
	openView := tasksToDto([]*model.Task{failed(taskFailUpstreamSecret)}, false, common.RoleCommonUser)[0]
	assert.Equal(t, taskFailUpstreamSecret, openView.FailReason)
}
