package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/gatewaylimit"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
)

func GetGatewayLimits(c *gin.Context) {
	mode := "memory"
	if common.RedisEnabled {
		mode = "redis"
	}
	common.ApiSuccess(c, gin.H{"config": gatewaylimit.Current().Config, "backend": mode, "legacy_enabled": setting.ModelRequestRateLimitEnabled})
}
func UpdateGatewayLimits(c *gin.Context) {
	var config gatewaylimit.Config
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid rate limit configuration"})
		return
	}
	if err := model.SaveGatewayLimits(config); err != nil {
		code := http.StatusInternalServerError
		message := "Failed to save rate limit rules"
		if errors.Is(err, model.ErrGatewayLimitConflict) {
			code = http.StatusConflict
			message = err.Error()
		} else if errors.Is(err, model.ErrGatewayLimitInvalid) {
			code = http.StatusBadRequest
			message = err.Error()
		} else {
			logger.LogError(c.Request.Context(), "save gateway limits: "+err.Error())
		}
		c.JSON(code, gin.H{"success": false, "message": message})
		return
	}
	GetGatewayLimits(c)
}
func GetGatewayLimitUsage(c *gin.Context) {
	ids := strings.Split(c.Query("ids"), ",")
	if len(ids) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Too many rate limit rules"})
		return
	}
	wanted := map[string]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	rules := make([]gatewaylimit.Rule, 0, len(ids))
	for _, r := range gatewaylimit.Current().Config.Rules {
		if wanted[r.ID] {
			rules = append(rules, r)
		}
	}
	usage, err := gatewaylimit.Default.Inspect(c.Request.Context(), rules)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "Rate limit storage unavailable"})
		return
	}
	common.ApiSuccess(c, usage)
}

// Targets deliberately select metadata only: API credentials never reach this API.
func GetGatewayLimitTargets(c *gin.Context) {
	kind := c.Query("kind")
	term := strings.TrimSpace(c.Query("search"))
	if len(term) > 64 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Search is too long"})
		return
	}
	type target struct {
		ID     int    `json:"id"`
		Name   string `json:"name"`
		UserID int    `json:"user_id,omitempty"`
	}
	var results []target
	var dbModel any
	var columns, nameColumn string
	switch kind {
	case "user":
		dbModel = &model.User{}
		columns = "id, username AS name"
		nameColumn = "username"
	case "api_key":
		dbModel = &model.Token{}
		columns = "id, name, user_id"
		nameColumn = "name"
	case "provider":
		dbModel = &model.Channel{}
		columns = "id, name"
		nameColumn = "name"
	default:
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid target type"})
		return
	}
	query := model.DB.Model(dbModel).Select(columns)
	if id, err := strconv.Atoi(term); err == nil {
		query = query.Where("id = ?", id)
	} else if term != "" {
		term = strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(strings.ToLower(term))
		query = query.Where("LOWER("+nameColumn+") LIKE ? ESCAPE '!'", "%"+term+"%")
	}
	if kind == "api_key" && c.Query("user_id") != "" {
		userID, err := strconv.Atoi(c.Query("user_id"))
		if err != nil || userID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid user ID"})
			return
		}
		query = query.Where("user_id = ?", userID)
	}
	if err := query.Order("id DESC").Limit(30).Find(&results).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to load rate limit targets"})
		return
	}
	if results == nil {
		results = []target{}
	}
	common.ApiSuccess(c, results)
}
