package service

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/adaptiverouting"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting"
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
	priority := int64(0)
	weight := uint(100)
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

func TestAdaptiveRoutingIsOptInAndPreservesModelAndPriorityBoundaries(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	oldStore := adaptiverouting.Default
	adaptiverouting.Default = adaptiverouting.NewStore()
	t.Cleanup(func() { adaptiverouting.Default = oldStore })
	createChannelSelectAutoGroupsChannel(t, db, 3101, "default", "glm-5.3")
	createChannelSelectAutoGroupsChannel(t, db, 3102, "default", "glm-5.3")
	createChannelSelectAutoGroupsChannel(t, db, 3103, "vip", "glm-5.3")
	model.InitChannelCache()
	now := time.Now()
	for _, id := range []int{3101, 3103} {
		adaptiverouting.Default.Record(adaptiverouting.Sample{ChannelID: id, Model: "glm-5.3", Outcome: "failure", Status: 429, RetryAfter: "60"}, now)
	}
	for _, cached := range []bool{true, false} {
		t.Run(fmt.Sprint(cached), func(t *testing.T) {
			common.MemoryCacheEnabled = cached
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
			common.SetContextKey(ctx, constant.ContextKeyUserSetting, dto.UserSetting{AdaptiveRouting: true})
			selected, group, err := CacheGetRandomSatisfiedChannel(&RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: "glm-5.3"})
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Equal(t, 3102, selected.Id)
			assert.Equal(t, "default", group)
			common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"vip", "default"})
			selected, group, err = CacheGetRandomSatisfiedChannel(&RetryParam{Ctx: ctx, TokenGroup: "auto", ModelName: "glm-5.3"})
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Equal(t, 3102, selected.Id)
			assert.Equal(t, "default", group, "a rate-limited auto group can fall back to the next allowed group")
			// A disabled user still uses the original selector, even when all routes
			// have adaptive health penalties.
			adaptiverouting.Default.Record(adaptiverouting.Sample{ChannelID: 3102, Model: "glm-5.3", Outcome: "failure", Status: 429, RetryAfter: "60"}, now)
			common.SetContextKey(ctx, constant.ContextKeyUserSetting, dto.UserSetting{})
			selected, _, err = CacheGetRandomSatisfiedChannel(&RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: "glm-5.3"})
			require.NoError(t, err)
			require.NotNil(t, selected)
			common.SetContextKey(ctx, constant.ContextKeyUserSetting, dto.UserSetting{AdaptiveRouting: true})
			_, _, err = CacheGetRandomSatisfiedChannel(&RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: "glm-5.3"})
			var limited *adaptiverouting.RateLimitedError
			require.ErrorAs(t, err, &limited)
			// Reset between cache paths without changing configured channel weights.
			adaptiverouting.Default = adaptiverouting.NewStore()
			for _, id := range []int{3101, 3103} {
				adaptiverouting.Default.Record(adaptiverouting.Sample{ChannelID: id, Model: "glm-5.3", Outcome: "failure", Status: 429, RetryAfter: "60"}, now)
			}
		})
	}
}
