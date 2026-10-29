package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPublicErrorsUseGenericType(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		typeName string
		handler  gin.HandlerFunc
	}{
		{name: "unimplemented", status: http.StatusNotImplemented, handler: RelayNotImplemented},
		{name: "unknown URL", status: http.StatusNotFound, typeName: "invalid_request_error", handler: RelayNotFound},
		{name: "plugin protocol", status: http.StatusInternalServerError, handler: func(c *gin.Context) {
			respondPluginProtocolError(c, http.StatusInternalServerError, "task_protocol_error", "Task protocol request failed")
		}},
		{name: "plugin image", status: http.StatusBadGateway, handler: func(c *gin.Context) {
			respondTaskPluginImageError(c, &dto.TaskError{StatusCode: http.StatusBadGateway, Code: "upstream_failure", Message: "Upstream failed"})
		}},
		{name: "local OpenAI error", status: http.StatusBadRequest, handler: func(c *gin.Context) {
			err := types.NewErrorWithStatusCode(errors.New("Invalid request"), types.ErrorCodeInvalidRequest, http.StatusBadRequest)
			c.JSON(err.StatusCode, gin.H{"error": err.ToOpenAIError()})
		}},
		{name: "local Claude error", status: http.StatusBadRequest, handler: func(c *gin.Context) {
			err := types.NewErrorWithStatusCode(errors.New("Invalid request"), types.ErrorCodeInvalidRequest, http.StatusBadRequest)
			c.JSON(err.StatusCode, gin.H{"error": err.ToClaudeError()})
		}},
		{name: "Claude wrapper", status: http.StatusBadGateway, handler: func(c *gin.Context) {
			err := service.ClaudeErrorWrapper(errors.New("Upstream failed"), "upstream_failure", http.StatusBadGateway)
			c.JSON(err.StatusCode, gin.H{"error": err.Error})
		}},
		{name: "legacy upstream OpenAI type", status: http.StatusBadGateway, handler: func(c *gin.Context) {
			err := types.WithOpenAIError(types.OpenAIError{Type: "new_api_error", Message: "Upstream failed", Code: "upstream_failure"}, http.StatusBadGateway)
			c.JSON(err.StatusCode, gin.H{"error": err.ToOpenAIError()})
		}},
		{name: "legacy upstream Claude type", status: http.StatusBadGateway, handler: func(c *gin.Context) {
			err := types.WithClaudeError(types.ClaudeError{Type: "new_api_error", Message: "Upstream failed"}, http.StatusBadGateway)
			c.JSON(err.StatusCode, gin.H{"error": err.ToClaudeError()})
		}},
		{name: "legacy Claude to OpenAI", status: http.StatusBadGateway, handler: func(c *gin.Context) {
			err := types.WithClaudeError(types.ClaudeError{Type: "new_api_error", Message: "Upstream failed"}, http.StatusBadGateway)
			c.JSON(err.StatusCode, gin.H{"error": err.ToOpenAIError()})
		}},
		{name: "standard upstream type", status: http.StatusTooManyRequests, typeName: "rate_limit_error", handler: func(c *gin.Context) {
			err := types.WithOpenAIError(types.OpenAIError{Type: "rate_limit_error", Message: "Too many requests", Code: "rate_limit_exceeded"}, http.StatusTooManyRequests)
			c.JSON(err.StatusCode, gin.H{"error": err.ToOpenAIError()})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			tc.handler(c)
			require.Equal(t, tc.status, recorder.Code)
			var response struct {
				Error types.OpenAIError `json:"error"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			wantType := tc.typeName
			if wantType == "" {
				wantType = "api_error"
			}
			assert.Equal(t, wantType, response.Error.Type)
			assert.NotEmpty(t, response.Error.Message)
			assert.NotContains(t, recorder.Body.String(), "new_api_error")
		})
	}
}

func TestTokenFailuresKeepGenericErrorAndAbort(t *testing.T) {
	require.NoError(t, i18n.Init())
	previousDB, previousRedis := model.DB, common.RedisEnabled
	initModelListColumnNames(t)
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate(&model.Token{}))
	model.DB, common.RedisEnabled = database, false
	t.Cleanup(func() {
		model.DB, common.RedisEnabled = previousDB, previousRedis
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, database.Create(&model.Token{Key: "expiredfixture", Status: common.TokenStatusExpired, ExpiredTime: 1}).Error)
	for _, authorization := range []string{"", "Bearer invalidfixture", "Bearer expiredfixture"} {
		t.Run(authorization, func(t *testing.T) {
			router := gin.New()
			router.POST("/v1/chat/completions", middleware.RequestId(), middleware.TokenAuth(), func(c *gin.Context) {
				t.Error("invalid tokens must not reach the relay handler")
				c.Status(http.StatusNoContent)
			})
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			request.Header.Set("Authorization", authorization)
			request.Header.Set("Accept-Language", "en")
			router.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusUnauthorized, recorder.Code)
			requestID := recorder.Header().Get(common.RequestIdKey)
			require.NotEmpty(t, requestID)
			assert.JSONEq(t, `{"error":{"type":"api_error","code":"","message":"Invalid token (request id: `+requestID+`)"}}`, recorder.Body.String())
		})
	}
}

func TestProcessChannelErrorUsesSnapshotWithoutLeakingChannelMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	previousErrorLogEnabled := constant.ErrorLogEnabled

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.Log{}))
	model.DB, model.LOG_DB = database, database
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	constant.ErrorLogEnabled = true
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedisEnabled
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		constant.ErrorLogEnabled = previousErrorLogEnabled
		require.NoError(t, sqlDB.Close())
	})

	require.NoError(t, database.Create(&model.User{Id: 7, Username: "log-owner", Group: "default"}).Error)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set("id", 7)
	ctx.Set("username", "log-owner")
	ctx.Set("token_name", "test-token")
	ctx.Set("token_id", 11)
	ctx.Set("original_model", "gpt-test")
	ctx.Set("group", "default")
	ctx.Set("channel_id", 202)
	ctx.Set("channel_name", "mutable-context-channel")
	ctx.Set("channel_type", 9)
	ctx.Set("use_channel", []string{"101"})
	common.SetContextKey(ctx, constant.ContextKeyRequestStartTime, time.Now().Add(-time.Second))

	channelSnapshot := types.ChannelError{
		ChannelId:   101,
		ChannelType: 1,
		ChannelName: "snapshot-channel",
		AutoBan:     false,
	}
	apiErr := types.NewOpenAIError(errors.New("upstream failed"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)

	processChannelError(ctx, channelSnapshot, apiErr, nil)

	var stored model.Log
	require.NoError(t, database.First(&stored).Error)
	assert.Equal(t, channelSnapshot.ChannelId, stored.ChannelId)
	storedOther, err := common.StrToMap(stored.Other)
	require.NoError(t, err)
	assert.Equal(t, float64(http.StatusBadGateway), storedOther["status_code"])
	for _, key := range []string{"channel_id", "channel_name", "channel_type"} {
		assert.NotContains(t, storedOther, key)
	}
	adminInfo, ok := storedOther["admin_info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []any{"101"}, adminInfo["use_channel"])

	logs, total, err := model.GetUserLogs(7, model.LogTypeError, 0, 0, "", "", 0, 10, "", "", "")
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	assert.Equal(t, channelSnapshot.ChannelId, logs[0].ChannelId)
	assert.Empty(t, logs[0].ChannelName)
	userOther, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	assert.NotContains(t, userOther, "admin_info")
	for _, key := range []string{"channel_id", "channel_name", "channel_type"} {
		assert.NotContains(t, userOther, key)
	}
}
