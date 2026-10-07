package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelStatusTest(t *testing.T) {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)

	memoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		common.MemoryCacheEnabled = memoryCacheEnabled
	})
}

func TestUpdateChannelStatusPersistsMultiKeyState(t *testing.T) {
	setupChannelStatusTest(t)

	channel := Channel{
		Name:   "multi-key-status",
		Key:    "key-a\nkey-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey:           true,
			MultiKeySize:         2,
			MultiKeyMode:         constant.MultiKeyModePolling,
			MultiKeyPollingIndex: 1,
		},
	}
	require.NoError(t, DB.Create(&channel).Error)

	changed := UpdateChannelStatus(channel.Id, "key-a", common.ChannelStatusAutoDisabled, "provider rejected key")
	require.True(t, changed)

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.ChannelInfo.MultiKeyStatusList[0])
	assert.Equal(t, "provider rejected key", stored.ChannelInfo.MultiKeyDisabledReason[0])
	assert.NotZero(t, stored.ChannelInfo.MultiKeyDisabledTime[0])
	assert.Equal(t, 1, stored.ChannelInfo.MultiKeyPollingIndex)
}

func TestSaveStatusStateFromSingleKeySnapshotPreservesUnownedColumns(t *testing.T) {
	setupChannelStatusTest(t)

	channel := Channel{
		Name:        "single-key-status",
		Key:         "original-key",
		Status:      common.ChannelStatusEnabled,
		Models:      "original-model",
		Group:       "default",
		UsedQuota:   100,
		ChannelInfo: ChannelInfo{},
	}
	require.NoError(t, DB.Create(&channel).Error)

	stale, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)

	concurrentChannelInfo := ChannelInfo{
		IsMultiKey:           true,
		MultiKeySize:         2,
		MultiKeyMode:         constant.MultiKeyModePolling,
		MultiKeyPollingIndex: 1,
	}
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{
		"key":          "rotated-key",
		"used_quota":   gorm.Expr("used_quota + ?", 250),
		"models":       "concurrent-model",
		"channel_info": concurrentChannelInfo,
	}).Error)

	stale.Status = common.ChannelStatusManuallyDisabled
	stale.SetOtherInfo(map[string]any{
		"status_reason": "manual operation",
		"status_time":   int64(1234),
	})
	require.NoError(t, stale.saveStatusState())

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
	assert.Equal(t, "rotated-key", stored.Key)
	assert.Equal(t, int64(350), stored.UsedQuota)
	assert.Equal(t, "concurrent-model", stored.Models)
	assert.Equal(t, concurrentChannelInfo, stored.ChannelInfo)

	otherInfo := stored.GetOtherInfo()
	assert.Equal(t, "manual operation", otherInfo["status_reason"])
	assert.Equal(t, float64(1234), otherInfo["status_time"])
}

func TestGetChannelSkipsDisabledChannelDespiteStaleAbility(t *testing.T) {
	setupChannelStatusTest(t)

	channel := Channel{
		Name:   "zombie-channel",
		Key:    "key",
		Status: common.ChannelStatusManuallyDisabled,
		Models: "stale-model",
		Group:  "default",
	}
	require.NoError(t, DB.Create(&channel).Error)
	// Simulate the historical inconsistency: the abilities row stayed enabled
	// while the channel itself is disabled.
	staleAbility := Ability{
		Group:     "default",
		Model:     "stale-model",
		ChannelId: channel.Id,
		Enabled:   true,
		Priority:  common.GetPointer[int64](0),
		Weight:    10,
	}
	require.NoError(t, DB.Create(&staleAbility).Error)

	selected, err := GetChannel([]string{"default"}, "stale-model", 0, nil)
	require.NoError(t, err)
	assert.Nil(t, selected, "a disabled channel must not be selected even with a stale enabled ability row")

	// A healthy enabled channel with the same model stays selectable.
	healthy := Channel{
		Name:   "healthy-channel",
		Key:    "key",
		Status: common.ChannelStatusEnabled,
		Models: "stale-model",
		Group:  "default",
	}
	require.NoError(t, DB.Create(&healthy).Error)
	require.NoError(t, DB.Create(&Ability{
		Group:     "default",
		Model:     "stale-model",
		ChannelId: healthy.Id,
		Enabled:   true,
		Priority:  common.GetPointer[int64](0),
		Weight:    10,
	}).Error)

	selected, err = GetChannel([]string{"default"}, "stale-model", 0, nil)
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, healthy.Id, selected.Id)
}

func TestUpdateChannelStatusRepairsStaleAbilitiesOnIdempotentDisable(t *testing.T) {
	setupChannelStatusTest(t)

	channel := Channel{
		Name:   "repair-channel",
		Key:    "key",
		Status: common.ChannelStatusEnabled,
		Models: "repair-model",
		Group:  "default",
	}
	require.NoError(t, DB.Create(&channel).Error)
	ability := Ability{
		Group:     "default",
		Model:     "repair-model",
		ChannelId: channel.Id,
		Enabled:   true,
		Priority:  common.GetPointer[int64](0),
		Weight:    10,
	}
	require.NoError(t, DB.Create(&ability).Error)

	require.True(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusAutoDisabled, "provider rejected"))

	// Simulate a lost ability update so the row goes stale while the channel
	// is already disabled.
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", channel.Id).Update("enabled", true).Error)

	// The repeated idempotent disable must repair the stale row instead of
	// returning early, otherwise the channel stays schedulable forever.
	changed := UpdateChannelStatus(channel.Id, "", common.ChannelStatusAutoDisabled, "provider rejected")
	require.False(t, changed)

	var stored Ability
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).First(&stored).Error)
	assert.False(t, stored.Enabled)
}

// Channel model and group lists are stored as comma-separated text but indexed
// verbatim by the abilities table and the memory cache, so an untrimmed entry
// makes a channel unroutable for that name. Selection looks them up with the
// name the client sent, so both sides must agree on the trimmed form.
func TestAbilityRowsUseTrimmedModelAndGroupNames(t *testing.T) {
	setupChannelStatusTest(t)

	channel := Channel{
		Name:   "untrimmed-lists",
		Key:    "key",
		Status: common.ChannelStatusEnabled,
		Models: "alpha, beta ,  gamma",
		Group:  "default, vip",
	}
	require.NoError(t, DB.Create(&channel).Error)
	require.NoError(t, channel.UpdateAbilities(nil))

	var abilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Order("`group`, model").Find(&abilities).Error)

	var got []string
	for _, ability := range abilities {
		got = append(got, ability.Group+"|"+ability.Model)
	}
	assert.Equal(t, []string{
		"default|alpha",
		"default|beta",
		"default|gamma",
		"vip|alpha",
		"vip|beta",
		"vip|gamma",
	}, got)

	// Each name must resolve back to the channel on the DB selection path.
	for _, modelName := range []string{"alpha", "beta", "gamma"} {
		for _, group := range []string{"default", "vip"} {
			selected, err := GetChannel([]string{group}, modelName, 0, nil)
			require.NoError(t, err, "group=%s model=%s", group, modelName)
			require.NotNil(t, selected, "group=%s model=%s must resolve to the channel", group, modelName)
			assert.Equal(t, channel.Id, selected.Id)
		}
	}
}

func TestGetModelsAndGetGroupsDropEmptyEntries(t *testing.T) {
	channel := Channel{Models: "alpha,,beta, ", Group: ",default,,vip,"}

	assert.Equal(t, []string{"alpha", "beta"}, channel.GetModels())
	assert.Equal(t, []string{"default", "vip"}, channel.GetGroups())
}

const (
	normalizationTestChannelID = 4101
	normalizationTestTag       = "normalization-tag"
	// Group and Models exactly as an admin form or an import can submit them:
	// padded names, an empty segment and a trailing comma.
	paddedChannelGroup  = " vip , default ,"
	paddedChannelModels = " gpt-4 ,, gpt-4o, "
)

func newNormalizationTestChannel(group, models string) *Channel {
	tag := normalizationTestTag
	return &Channel{
		Id:     normalizationTestChannelID,
		Name:   "normalization-channel",
		Key:    "key",
		Status: common.ChannelStatusEnabled,
		Group:  group,
		Models: models,
		Tag:    &tag,
	}
}

func loadAbilityPairs(t *testing.T, channelID int) []string {
	t.Helper()
	var abilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", channelID).Find(&abilities).Error)
	pairs := make([]string, 0, len(abilities))
	for _, ability := range abilities {
		pairs = append(pairs, ability.Group+"|"+ability.Model)
	}
	return pairs
}

func TestChannelGroupAndModelsAccessorsNormalizePaddedValues(t *testing.T) {
	tests := []struct {
		raw  string
		want []string
	}{
		{raw: "", want: []string{}},
		{raw: "default", want: []string{"default"}},
		{raw: "vip,default", want: []string{"vip", "default"}},
		{raw: "gpt-4,gpt-4o-mini", want: []string{"gpt-4", "gpt-4o-mini"}},
		{raw: paddedChannelGroup, want: []string{"vip", "default"}},
		{raw: ",vip,,default", want: []string{"vip", "default"}},
		{raw: " , ", want: []string{}},
	}
	for _, tt := range tests {
		channel := Channel{Group: tt.raw, Models: tt.raw}
		assert.Equal(t, tt.want, channel.GetGroups(), "groups of %q", tt.raw)
		assert.Equal(t, tt.want, channel.GetModels(), "models of %q", tt.raw)
	}

	// Well-formed values are already canonical and must round-trip unchanged.
	for _, raw := range []string{"default", "vip,default", "gpt-4,gpt-4o-mini"} {
		assert.Equal(t, raw, normalizeCommaSeparated(raw))
	}
}

func TestChannelWritePathsPersistCanonicalGroupAndModels(t *testing.T) {
	tests := []struct {
		name string
		// seed stores a canonical channel first so the path edits an existing row.
		seed bool
		// syncsAbilities is false for Save, which persists the channel row only.
		syncsAbilities bool
		write          func() error
	}{
		{name: "Insert", syncsAbilities: true, write: func() error {
			return newNormalizationTestChannel(paddedChannelGroup, paddedChannelModels).Insert()
		}},
		{name: "BatchInsertChannels", syncsAbilities: true, write: func() error {
			return BatchInsertChannels([]Channel{*newNormalizationTestChannel(paddedChannelGroup, paddedChannelModels)})
		}},
		{name: "Update", seed: true, syncsAbilities: true, write: func() error {
			return newNormalizationTestChannel(paddedChannelGroup, paddedChannelModels).Update()
		}},
		{name: "Save", seed: true, write: func() error {
			return newNormalizationTestChannel(paddedChannelGroup, paddedChannelModels).Save()
		}},
		{name: "EditChannelByTag", seed: true, syncsAbilities: true, write: func() error {
			group, models := paddedChannelGroup, paddedChannelModels
			return EditChannelByTag(normalizationTestTag, nil, nil, &models, &group, nil, nil, nil, nil)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupChannelStatusTest(t)
			if tt.seed {
				require.NoError(t, newNormalizationTestChannel("default", "seed-model").Insert())
			}
			require.NoError(t, tt.write())

			var stored Channel
			require.NoError(t, DB.First(&stored, normalizationTestChannelID).Error)
			assert.Equal(t, "vip,default", stored.Group)
			assert.Equal(t, "gpt-4,gpt-4o", stored.Models)
			if tt.syncsAbilities {
				assert.ElementsMatch(t,
					[]string{"vip|gpt-4", "vip|gpt-4o", "default|gpt-4", "default|gpt-4o"},
					loadAbilityPairs(t, normalizationTestChannelID))
			}
		})
	}
}

func TestEditChannelByTagIgnoresBlankGroupAndModels(t *testing.T) {
	setupChannelStatusTest(t)
	require.NoError(t, newNormalizationTestChannel("default", "seed-model").Insert())

	// A value with nothing left after normalization counts as not provided;
	// stored verbatim it would strip every channel of the tag from routing.
	blank := " , "
	require.NoError(t, EditChannelByTag(normalizationTestTag, nil, nil, &blank, &blank, nil, nil, nil, nil))

	var stored Channel
	require.NoError(t, DB.First(&stored, normalizationTestChannelID).Error)
	assert.Equal(t, "default", stored.Group)
	assert.Equal(t, "seed-model", stored.Models)
	assert.Equal(t, []string{"default|seed-model"}, loadAbilityPairs(t, normalizationTestChannelID))
}

func TestInitChannelCacheIndexesLegacyPaddedChannelByTrimmedNames(t *testing.T) {
	setupChannelStatusTest(t)
	previousMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = previousMemoryCache })

	// A legacy row: hooks are skipped so the padded values persist raw, and the
	// only ability row is the padded one the pre-normalization code wrote. The
	// "default" group has no ability row at all.
	legacy := newNormalizationTestChannel(paddedChannelGroup, paddedChannelModels)
	require.NoError(t, DB.Session(&gorm.Session{SkipHooks: true}).Create(legacy).Error)
	require.NoError(t, DB.Create(&Ability{Group: " vip ", Model: " gpt-4 ", ChannelId: legacy.Id, Enabled: true}).Error)
	var raw Channel
	require.NoError(t, DB.First(&raw, legacy.Id).Error)
	require.Equal(t, paddedChannelGroup, raw.Group, "the fixture must hold the raw legacy value")

	wantIndexed := func(t *testing.T) {
		t.Helper()
		for _, group := range []string{"vip", "default"} {
			for _, modelName := range []string{"gpt-4", "gpt-4o"} {
				assert.True(t, IsChannelEnabledForGroupModel(group, modelName, legacy.Id), "%s/%s", group, modelName)
			}
		}
		assert.False(t, IsChannelEnabledForGroupModel(" vip ", " gpt-4 ", legacy.Id))
	}

	// The rebuild must create the index entry of a group that has no ability
	// row instead of panicking, and key everything on the trimmed names.
	require.NotPanics(t, InitChannelCache)
	wantIndexed(t)

	// FixAbility, the self-heal path, rebuilds the ability rows from the same
	// accessors, so abilities and index agree on the trimmed names as well.
	_, failed, err := FixAbility()
	require.NoError(t, err)
	require.Zero(t, failed)
	assert.ElementsMatch(t,
		[]string{"vip|gpt-4", "vip|gpt-4o", "default|gpt-4", "default|gpt-4o"},
		loadAbilityPairs(t, legacy.Id))
	wantIndexed(t)
}
