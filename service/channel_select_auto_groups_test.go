package service

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelSelectAutoGroupsTest(t *testing.T) *gorm.DB {
	t.Helper()

	originalDB := model.DB
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalRetryTimes := common.RetryTimes
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	originalMaxTokenAutoGroups := setting.GetMaxTokenAutoGroups()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	model.DB = db
	common.MemoryCacheEnabled = true
	common.RetryTimes = 0

	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`[]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2}`))
	require.NoError(t, setting.UpdateMaxTokenAutoGroups("2"))

	t.Cleanup(func() {
		model.DB = originalDB
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		common.RetryTimes = originalRetryTimes
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
		require.NoError(t, setting.UpdateMaxTokenAutoGroups(fmt.Sprintf("%d", originalMaxTokenAutoGroups)))

		if originalMemoryCacheEnabled && originalDB != nil &&
			originalDB.Migrator().HasTable(&model.Channel{}) && originalDB.Migrator().HasTable(&model.Ability{}) {
			model.InitChannelCache()
		}
		sqlDB, err := db.DB()
		if err == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	return db
}

func createChannelSelectAutoGroupsChannel(t *testing.T, db *gorm.DB, id int, group, modelName string) {
	t.Helper()
	createChannelSelectAutoGroupsChannelAt(t, db, id, group, modelName, 0, 100)
}

func createChannelSelectAutoGroupsChannelAt(t *testing.T, db *gorm.DB, id int, group, modelName string, priority int64, weight uint) {
	t.Helper()
	require.NoError(t, db.Create(&model.Channel{
		Id:       id,
		Type:     constant.ChannelTypeOpenAI,
		Key:      fmt.Sprintf("key-%d", id),
		Status:   common.ChannelStatusEnabled,
		Name:     fmt.Sprintf("channel-%d", id),
		Weight:   &weight,
		Models:   modelName,
		Group:    group,
		Priority: &priority,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     group,
		Model:     modelName,
		ChannelId: id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
}

func TestCacheGetRandomSatisfiedChannelUsesTokenAutoGroupsWhenGlobalAutoIsEmpty(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-groups-runtime-model"
	createChannelSelectAutoGroupsChannel(t, db, 2101, "vip", modelName)
	createChannelSelectAutoGroupsChannel(t, db, 2102, "default", modelName)
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"vip", "default"})
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)

	retry := 0
	param := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "auto",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       &retry,
	}

	first, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 2101, first.Id)
	assert.Equal(t, "vip", selectedGroup)
	assert.Equal(t, "vip", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
	assert.Empty(t, setting.GetAutoGroups(), "the selection must not depend on the global Auto list")

	param.IncreaseRetry()
	second, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, 2102, second.Id)
	assert.Equal(t, "default", selectedGroup)
	assert.Equal(t, "default", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
}

func TestCacheGetRandomSatisfiedChannelServesErrorRateCooldownChannelAsLastResort(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "cooldown-last-resort-model"
	createChannelSelectAutoGroupsChannel(t, db, 2201, "default", modelName)
	model.InitChannelCache()

	channelCooldownStatesMu.Lock()
	channelCooldownStates = make(map[channelModelKey]*channelModelState)
	channelCooldownStatesMu.Unlock()
	t.Cleanup(func() {
		channelCooldownStatesMu.Lock()
		channelCooldownStates = make(map[channelModelKey]*channelModelState)
		channelCooldownStatesMu.Unlock()
	})

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")

	newParam := func() *RetryParam {
		retry := 0
		return &RetryParam{
			Ctx:         ctx,
			TokenGroup:  "default",
			ModelName:   modelName,
			RequestPath: "/v1/chat/completions",
			Retry:       &retry,
		}
	}

	// Five consecutive failures arm the per-(channel, model) cooldown.
	for i := 0; i < 5; i++ {
		RecordChannelAttemptOutcome(2201, modelName, true)
	}
	require.True(t, ChannelInErrorCooldown(2201, modelName))

	// The cooled channel is the only candidate: skipping it would strand the
	// request, so it still serves.
	param := newParam()
	channel, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 2201, channel.Id)

	// Once another healthy channel exists, the cooled one is skipped again.
	createChannelSelectAutoGroupsChannel(t, db, 2202, "default", modelName)
	model.InitChannelCache()
	channel, _, err = CacheGetRandomSatisfiedChannel(newParam())
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 2202, channel.Id)
}

// setupModelOperatorSetting snapshots the model operator setting globals so a
// test can mutate them and restore on cleanup.
func setupModelOperatorSetting(t *testing.T) {
	t.Helper()
	operatorSetting := operation_setting.GetModelOperatorSetting()
	originalEnabled := operatorSetting.Enabled
	originalMap := operatorSetting.ModelChannelMap
	t.Cleanup(func() {
		operatorSetting.Enabled = originalEnabled
		operatorSetting.ModelChannelMap = originalMap
	})
}

func setModelOperator(t *testing.T, channelMap map[string]int) {
	t.Helper()
	operatorSetting := operation_setting.GetModelOperatorSetting()
	operatorSetting.Enabled = true
	operatorSetting.ModelChannelMap = channelMap
}

func TestCacheGetRandomSatisfiedChannelRoutesModelToSoleOperatorChannel(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	setupModelOperatorSetting(t)
	const modelName = "sole-operator-model"
	// The operator channel sits in the lowest priority tier while two
	// competitors outrank it, so default routing could never pick it first and
	// any selection of it proves the mapping pinned the request.
	createChannelSelectAutoGroupsChannelAt(t, db, 2302, "default", modelName, 10, 100)
	createChannelSelectAutoGroupsChannelAt(t, db, 2303, "default", modelName, 20, 100)
	createChannelSelectAutoGroupsChannelAt(t, db, 2301, "default", modelName, 0, 10)
	setModelOperator(t, map[string]int{modelName: 2301})
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	retry := 0
	param := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "default",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       &retry,
	}

	// First attempt: the mapping overrides the priority order entirely.
	channel, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 2301, channel.Id)

	// Retries no longer re-pin the operator: the default priority logic moves
	// to the next tier instead of falling back onto the operator channel.
	param.IncreaseRetry()
	channel, _, err = CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 2302, channel.Id)
}

func TestCacheGetRandomSatisfiedChannelFallsBackWhenSoleOperatorUnusable(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	setupModelOperatorSetting(t)
	const modelName = "sole-operator-fallback-model"
	createChannelSelectAutoGroupsChannel(t, db, 2401, "default", modelName)
	createChannelSelectAutoGroupsChannel(t, db, 2402, "default", modelName)
	setModelOperator(t, map[string]int{modelName: 2401})
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	newParam := func() *RetryParam {
		retry := 0
		return &RetryParam{
			Ctx:         ctx,
			TokenGroup:  "default",
			ModelName:   modelName,
			RequestPath: "/v1/chat/completions",
			Retry:       &retry,
		}
	}

	// Operator channel disabled: fall back to the default routing logic.
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 2401).Update("status", common.ChannelStatusManuallyDisabled).Error)
	model.CacheUpdateChannelStatus(2401, common.ChannelStatusManuallyDisabled)
	channel, _, err := CacheGetRandomSatisfiedChannel(newParam())
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 2402, channel.Id)

	// Operator channel id pointing at a deleted/unknown channel: same fallback.
	setModelOperator(t, map[string]int{modelName: 999999})
	channel, _, err = CacheGetRandomSatisfiedChannel(newParam())
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Contains(t, []int{2401, 2402}, channel.Id)

	// Operator channel cooled down by failures: the mapping must not bypass the
	// cooldown, so the request falls back to the other channel.
	setModelOperator(t, map[string]int{modelName: 2401})
	for i := 0; i < 5; i++ {
		RecordChannelAttemptOutcome(2401, modelName, true)
	}
	require.True(t, ChannelInErrorCooldown(2401, modelName))
	channel, _, err = CacheGetRandomSatisfiedChannel(newParam())
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 2402, channel.Id)
}

func TestCacheGetRandomSatisfiedChannelSkipsSoleOperatorOutsideGroupPool(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	setupModelOperatorSetting(t)
	const modelName = "sole-operator-group-model"
	// The operator channel serves another group only, so pinning it would route
	// a request past its group pool and price it with the wrong group.
	createChannelSelectAutoGroupsChannel(t, db, 2601, "vip", modelName)
	createChannelSelectAutoGroupsChannel(t, db, 2602, "default", modelName)
	setModelOperator(t, map[string]int{modelName: 2601})
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	retry := 0
	param := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "default",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       &retry,
	}

	channel, selectGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 2602, channel.Id)
	assert.Equal(t, "default", selectGroup)
}

// Channels store models and groups as comma-separated text typed by hand, so a
// stray space after a comma used to end up inside the abilities rows and the
// memory-cache index, where the exact name the client sent can never match it.
// The whole selection pipeline must therefore key on the trimmed names.
func TestCacheGetRandomSatisfiedChannelMatchesTrimmedModelAndGroupNames(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	setupModelOperatorSetting(t)
	const modelName = "trimmed-name-model"

	createUntrimmedChannel := func(id int, priority int64, weight uint, groups string) {
		t.Helper()
		require.NoError(t, db.Create(&model.Channel{
			Id:       id,
			Type:     constant.ChannelTypeOpenAI,
			Key:      fmt.Sprintf("key-%d", id),
			Status:   common.ChannelStatusEnabled,
			Name:     fmt.Sprintf("untrimmed-channel-%d", id),
			Weight:   &weight,
			Models:   "trimmed-name-model , pad-model ",
			Group:    groups,
			Priority: &priority,
		}).Error)
		channel, err := model.GetChannelById(id, true)
		require.NoError(t, err)
		require.NoError(t, channel.UpdateAbilities(nil))
	}

	// Two groups written with a leading space each, plus a model list with a
	// trailing space on the name the client asks for.
	createUntrimmedChannel(2701, 10, 100, "default")
	createUntrimmedChannel(2702, 0, 100, " vip")
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	newParam := func(group string) *RetryParam {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
		retry := 0
		return &RetryParam{
			Ctx:         ctx,
			TokenGroup:  group,
			ModelName:   modelName,
			RequestPath: "/v1/chat/completions",
			Retry:       &retry,
		}
	}

	// The trimmed group name resolves to its own channel on the cache path.
	for _, group := range []string{"default", "vip"} {
		channel, selectGroup, err := CacheGetRandomSatisfiedChannel(newParam(group))
		require.NoError(t, err, "group=%s", group)
		require.NotNil(t, channel, "group=%s must resolve through the trimmed names", group)
		assert.Equal(t, group, selectGroup)
	}

	// The sole-operator gate looks the pinned channel up by exact group and model
	// before default routing runs, so an untrimmed row made the mapping silently
	// miss. The vip channel outranks nothing here, so pinning it must still win.
	setModelOperator(t, map[string]int{modelName: 2702})
	channel, selectGroup, err := CacheGetRandomSatisfiedChannel(newParam("vip"))
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 2702, channel.Id)
	assert.Equal(t, "vip", selectGroup)
}

func TestCacheGetRandomSatisfiedChannelIgnoresOperatorForUnmappedModel(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	setupModelOperatorSetting(t)
	const mappedModel = "sole-operator-mapped-model"
	const otherModel = "sole-operator-unmapped-model"
	createChannelSelectAutoGroupsChannel(t, db, 2501, "default", mappedModel)
	createChannelSelectAutoGroupsChannel(t, db, 2502, "default", otherModel)
	setModelOperator(t, map[string]int{mappedModel: 2501})
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	retry := 0
	param := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "default",
		ModelName:   otherModel,
		RequestPath: "/v1/chat/completions",
		Retry:       &retry,
	}

	// A model without a mapping keeps the default routing logic untouched.
	channel, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 2502, channel.Id)
}

// TestChannelSelectionAgreesOnTrimmedChannelGroup follows a channel whose Group
// carries stray spaces from storage to billing: the ability rows, the cache
// index, the selected group and the request context must all name "vip".
func TestChannelSelectionAgreesOnTrimmedChannelGroup(t *testing.T) {
	const (
		modelName    = "padded-group-model"
		vipChannelID = 2701
	)
	tests := []struct {
		name string
		// legacy stores the channel the way pre-normalization code did: padded
		// raw columns and a padded ability row, healed by FixAbility.
		legacy bool
		// auto routes through the token auto-group list instead of the user's
		// multi-group pool.
		auto            bool
		wantUsingGroup  string
		wantStoredGroup string
	}{
		{name: "inserted channel in user group pool", wantUsingGroup: "vip", wantStoredGroup: "vip"},
		{name: "inserted channel in auto groups", auto: true, wantUsingGroup: "auto", wantStoredGroup: "vip"},
		{name: "legacy channel in user group pool", legacy: true, wantUsingGroup: "vip", wantStoredGroup: " vip "},
		{name: "legacy channel in auto groups", legacy: true, auto: true, wantUsingGroup: "auto", wantStoredGroup: " vip "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupChannelSelectAutoGroupsTest(t)
			vipPriority, defaultPriority := int64(10), int64(0)
			weight := uint(100)
			vipChannel := &model.Channel{
				Id: vipChannelID, Type: constant.ChannelTypeOpenAI, Key: "key-vip", Status: common.ChannelStatusEnabled,
				Name: "vip-channel", Weight: &weight, Models: modelName, Group: " vip ", Priority: &vipPriority,
			}
			defaultChannel := &model.Channel{
				Id: vipChannelID + 1, Type: constant.ChannelTypeOpenAI, Key: "key-default", Status: common.ChannelStatusEnabled,
				Name: "default-channel", Weight: &weight, Models: modelName, Group: "default", Priority: &defaultPriority,
			}
			require.NoError(t, defaultChannel.Insert())
			if tt.legacy {
				require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(vipChannel).Error)
				require.NoError(t, db.Create(&model.Ability{
					Group: " vip ", Model: modelName, ChannelId: vipChannelID, Enabled: true, Priority: &vipPriority, Weight: weight,
				}).Error)
				_, failed, err := model.FixAbility()
				require.NoError(t, err)
				require.Zero(t, failed)
			} else {
				require.NoError(t, vipChannel.Insert())
				model.InitChannelCache()
			}

			var stored model.Channel
			require.NoError(t, db.First(&stored, vipChannelID).Error)
			assert.Equal(t, tt.wantStoredGroup, stored.Group)
			var abilities []model.Ability
			require.NoError(t, db.Where("channel_id = ?", vipChannelID).Find(&abilities).Error)
			require.Len(t, abilities, 1)
			assert.Equal(t, "vip", abilities[0].Group)
			assert.Equal(t, modelName, abilities[0].Model)
			assert.True(t, model.IsChannelEnabledForGroupModel("vip", modelName, vipChannelID))
			assert.False(t, model.IsChannelEnabledForGroupModel(" vip ", modelName, vipChannelID))

			gin.SetMode(gin.TestMode)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
			retry := 0
			param := &RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: modelName, RequestPath: "/v1/chat/completions", Retry: &retry}
			if tt.auto {
				param.TokenGroup = "auto"
				common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"vip", "default"})
				common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)
			} else {
				common.SetContextKey(ctx, constant.ContextKeyUserGroups, []string{"default", "vip"})
			}
			common.SetContextKey(ctx, constant.ContextKeyUsingGroup, param.TokenGroup)

			channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
			require.NoError(t, err)
			require.NotNil(t, channel)
			assert.Equal(t, vipChannelID, channel.Id, "the higher priority vip channel must be reachable by its trimmed group")
			assert.Equal(t, "vip", selectedGroup)
			assert.Equal(t, tt.wantUsingGroup, common.GetContextKeyString(ctx, constant.ContextKeyUsingGroup))
			// The price helper bills the auto-group context value over the using
			// group, so that value decides the group ratio the request pays.
			billingGroup := common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup)
			assert.Equal(t, "vip", billingGroup)
			assert.InDelta(t, 2.0, ratio_setting.GetGroupRatio(billingGroup), 1e-9)
		})
	}
}
