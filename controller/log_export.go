package controller

import (
	"net/http"
	"os"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type logExportResponse struct {
	TaskID      string                 `json:"task_id"`
	Status      model.SystemTaskStatus `json:"status"`
	Processed   int64                  `json:"processed"`
	Bytes       int64                  `json:"bytes"`
	CreatedAt   int64                  `json:"created_at"`
	ExpiresAt   int64                  `json:"expires_at"`
	Self        bool                   `json:"self"`
	Fields      []string               `json:"fields"`
	Mode        string                 `json:"mode"`
	Granularity string                 `json:"granularity,omitempty"`
	Timezone    string                 `json:"timezone,omitempty"`
	GroupBy     []string               `json:"group_by,omitempty"`
}

func logExportView(task *model.SystemTask, payload service.LogExportPayload) logExportResponse {
	if payload.Mode == "" {
		payload.Mode = "details"
	}
	var state service.LogExportState
	if task.Status == model.SystemTaskStatusSucceeded {
		_ = common.UnmarshalJsonStr(task.Result, &state)
	} else {
		_ = task.DecodeState(&state)
	}
	return logExportResponse{
		TaskID: task.TaskID, Status: task.Status, Processed: state.Processed, Bytes: state.Bytes,
		CreatedAt: task.CreatedAt, ExpiresAt: task.CreatedAt + int64(service.LogExportRetention/time.Second),
		Self: payload.Self, Fields: payload.Fields, Mode: payload.Mode, Granularity: payload.Granularity, Timezone: payload.Timezone, GroupBy: payload.GroupBy,
	}
}

func CreateLogExport(c *gin.Context) {
	var request struct {
		Scope       string                `json:"scope" binding:"required,oneof=self all"`
		Filter      model.LogExportFilter `json:"filter"`
		Fields      []string              `json:"fields"`
		Mode        string                `json:"mode"`
		Granularity string                `json:"granularity"`
		Timezone    string                `json:"timezone"`
		GroupBy     []string              `json:"group_by"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid export request"})
		return
	}
	if request.Scope == "all" && c.GetInt("role") < common.RoleAdminUser {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "admin access required"})
		return
	}
	payload := service.LogExportPayload{
		OwnerID: c.GetInt("id"), Role: c.GetInt("role"), Self: request.Scope == "self",
		Filter: request.Filter, Fields: request.Fields, Mode: request.Mode, Granularity: request.Granularity, Timezone: request.Timezone, GroupBy: request.GroupBy,
	}
	if err := service.ValidateLogExport(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
		return
	}
	task, err := service.StartLogExport(payload)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "unable to start export; another export may be running"})
		return
	}
	common.ApiSuccess(c, logExportView(task, payload))
}

func ListLogExports(c *gin.Context) {
	// This is a bounded recent-history view; never return other owners' payloads
	// or internal errors (which can contain database or filesystem details).
	tasks, _, err := model.ListSystemTasks(model.SystemTaskFilter{Type: service.LogExportTaskType}, 0, 100)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	responses := make([]logExportResponse, 0)
	for _, task := range tasks {
		var payload service.LogExportPayload
		if task.DecodePayload(&payload) != nil || payload.OwnerID != c.GetInt("id") {
			continue
		}
		if !payload.Self && c.GetInt("role") < common.RoleAdminUser {
			continue
		}
		responses = append(responses, logExportView(task, payload))
	}
	common.ApiSuccess(c, responses)
}

func DownloadLogExport(c *gin.Context) {
	task, err := model.GetSystemTaskByTaskID(c.Param("task_id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var payload service.LogExportPayload
	if task == nil || task.Type != service.LogExportTaskType || task.DecodePayload(&payload) != nil || payload.OwnerID != c.GetInt("id") || (!payload.Self && c.GetInt("role") < common.RoleAdminUser) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "export not found"})
		return
	}
	if task.Status != model.SystemTaskStatusSucceeded || common.GetTimestamp() >= task.CreatedAt+int64(service.LogExportRetention/time.Second) {
		c.JSON(http.StatusGone, gin.H{"success": false, "message": "export is not ready or has expired"})
		return
	}
	path := service.LogExportFile(task.TaskID)
	file, err := os.Open(path)
	if err != nil {
		c.JSON(http.StatusGone, gin.H{"success": false, "message": "export file unavailable; check shared export storage"})
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="usage-logs-`+task.TaskID+`.csv"`)
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	http.ServeContent(c.Writer, c.Request, "usage-logs.csv", info.ModTime(), file)
}
