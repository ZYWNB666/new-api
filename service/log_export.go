package service

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const LogExportTaskType = "log_export"
const LogExportRetention = 24 * time.Hour

type LogExportPayload struct {
	OwnerID int                   `json:"owner_id"`
	Role    int                   `json:"role"`
	Self    bool                  `json:"self"`
	Filter  model.LogExportFilter `json:"filter"`
	Fields  []string              `json:"fields"`
}

type LogExportState struct {
	Processed int64 `json:"processed"`
	Bytes     int64 `json:"bytes"`
}

// Enforce the file limit at the writer boundary, including a single oversized
// CSV record. Checking only at progress-flush boundaries can overshoot badly.
type logExportWriter struct {
	file      *os.File
	remaining int64
}

func (w *logExportWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > w.remaining {
		return 0, errors.New("export file limit exceeded; narrow the filters")
	}
	n, err := w.file.Write(data)
	w.remaining -= int64(n)
	return n, err
}

// These identifiers are also the CSV headers. Quota is exported as raw quota
// units, and use_time as seconds, without changing billing or display rounding.
func ValidateLogExport(payload *LogExportPayload) error {
	if payload.OwnerID <= 0 || (!payload.Self && payload.Role < common.RoleAdminUser) {
		return errors.New("invalid export scope")
	}
	if _, err := model.LogExportColumns(payload.Fields, payload.Self); err != nil {
		return err
	}
	f := &payload.Filter
	// The log page defaults its end time to now + 1 hour. Freeze the export
	// boundary at creation instead of including future traffic.
	f.EndTimestamp = min(f.EndTimestamp, common.GetTimestamp())
	if f.StartTimestamp <= 0 || f.EndTimestamp < f.StartTimestamp || f.EndTimestamp-f.StartTimestamp > 7*86400 {
		return errors.New("export requires a past time range of at most 7 days")
	}
	if f.Type < model.LogTypeUnknown || f.Type > model.LogTypeLogin || f.Channel < 0 {
		return errors.New("invalid export filters")
	}
	for _, value := range []string{f.ModelName, f.Username, f.TokenName, f.Group, f.RequestID, f.UpstreamRequestID} {
		if len(value) > 256 {
			return errors.New("export filter is too long")
		}
	}
	if payload.Self {
		f.UserID = payload.OwnerID
		f.Username = ""
		f.Channel = 0
	}
	return nil
}

func LogExportDirectory() string {
	return common.GetEnvOrDefaultString("LOG_EXPORT_DIR", "./data/log-exports")
}

func LogExportFile(taskID string) string {
	// Only server-generated task IDs may be turned into paths.
	if !strings.HasPrefix(taskID, "systask_") || strings.ContainsAny(taskID, `/\\.`) {
		return ""
	}
	return filepath.Join(LogExportDirectory(), taskID+".csv")
}

type LogExportHandler struct{}

func (LogExportHandler) Type() string { return LogExportTaskType }

func init() { RegisterSystemTaskHandler(LogExportHandler{}) }

func (LogExportHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	var payload LogExportPayload
	if err := task.DecodePayload(&payload); err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	if err := ValidateLogExport(&payload); err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	state, err := generateLogExport(ctx, task, runnerID, payload)
	if err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, state, ""); err != nil {
		// The status update may have committed before releasing the lease failed.
		// Keep the file; downloads still require a succeeded, owned task, and the
		// retention worker also cleans files whose tasks never reached success.
		logSystemTaskLockError(ctx, task, err)
	}
}

// Export cleanup is scheduled independently of new requests, and works on the
// shared export volume. Partial files from crashed workers expire as well.
type logExportCleanupHandler struct{}

func (logExportCleanupHandler) Type() string            { return "log_export_cleanup" }
func (logExportCleanupHandler) Enabled() bool           { return true }
func (logExportCleanupHandler) Interval() time.Duration { return time.Hour }
func (logExportCleanupHandler) NewPayload() any         { return nil }
func init()                                             { RegisterSystemTaskHandler(logExportCleanupHandler{}) }

func (logExportCleanupHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	entries, err := os.ReadDir(LogExportDirectory())
	if err != nil && !os.IsNotExist(err) {
		failSystemTask(task, runnerID, err)
		return
	}
	for _, entry := range entries {
		if ctx.Err() != nil {
			return
		}
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "systask_") || (!strings.HasSuffix(entry.Name(), ".csv") && !strings.HasSuffix(entry.Name(), ".part")) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if time.Since(info.ModTime()) > LogExportRetention {
			if err := os.Remove(filepath.Join(LogExportDirectory(), entry.Name())); err != nil && !os.IsNotExist(err) {
				failSystemTask(task, runnerID, err)
				return
			}
		}
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, nil, ""); err != nil {
		logSystemTaskLockError(ctx, task, err)
	}
}

func generateLogExport(ctx context.Context, task *model.SystemTask, runnerID string, payload LogExportPayload) (state LogExportState, err error) {
	path := LogExportFile(task.TaskID)
	if path == "" {
		return state, errors.New("invalid export task id")
	}
	if err = os.MkdirAll(LogExportDirectory(), 0700); err != nil {
		return state, err
	}
	file, err := os.OpenFile(path+".part", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return state, err
	}
	defer func() {
		_ = file.Close()
		if err != nil {
			_ = os.Remove(path + ".part")
		}
	}()
	_, err = io.WriteString(file, "\xef\xbb\xbf")
	if err != nil {
		return state, err
	}
	maxRows := max(1, common.GetEnvOrDefault("LOG_EXPORT_MAX_ROWS", 10000000))
	maxBytes := int64(min(10240, max(1, common.GetEnvOrDefault("LOG_EXPORT_MAX_MB", 1024)))) * 1024 * 1024
	writer := csv.NewWriter(&logExportWriter{file: file, remaining: maxBytes - 3})
	writer.UseCRLF = true
	if err = writer.Write(payload.Fields); err != nil {
		return state, err
	}
	rows, err := model.OpenLogExportRows(ctx, payload.Filter, payload.Fields)
	if err != nil {
		return state, err
	}
	defer rows.Close()
	for rows.Next() {
		if err = ctx.Err(); err != nil {
			return state, err
		}
		if state.Processed >= int64(maxRows) {
			return state, errors.New("export row limit exceeded; narrow the filters")
		}
		var log model.Log
		if err = model.LOG_DB.ScanRows(rows, &log); err != nil {
			return state, err
		}
		record := make([]string, len(payload.Fields))
		for i, field := range payload.Fields {
			var value string
			switch field {
			case "created_at":
				value = time.Unix(log.CreatedAt, 0).UTC().Format(time.RFC3339)
			case "request_id":
				value = log.RequestId
			case "upstream_request_id":
				value = log.UpstreamRequestId
			case "client_request_id":
				var metadata struct {
					ClientRequestID string `json:"client_request_id"`
				}
				_ = common.UnmarshalJsonStr(log.Other, &metadata)
				value = metadata.ClientRequestID
			case "type":
				value = strconv.Itoa(log.Type)
			case "model_name":
				value = log.ModelName
			case "token_name":
				value = log.TokenName
			case "prompt_tokens":
				value = strconv.Itoa(log.PromptTokens)
			case "completion_tokens":
				value = strconv.Itoa(log.CompletionTokens)
			case "quota":
				value = strconv.Itoa(log.Quota)
			case "use_time":
				value = strconv.Itoa(log.UseTime)
			case "is_stream":
				value = strconv.FormatBool(log.IsStream)
			case "group":
				value = log.Group
			case "ip":
				value = log.Ip
			case "content":
				value = log.Content
			case "user_id":
				value = strconv.Itoa(log.UserId)
			case "username":
				value = log.Username
			case "channel":
				value = strconv.Itoa(log.ChannelId)
			}
			// CSV quoting alone does not prevent spreadsheet formula execution.
			trimmed := strings.TrimLeftFunc(value, unicode.IsSpace)
			if strings.HasPrefix(value, "\t") || strings.HasPrefix(value, "\r") || strings.HasPrefix(value, "\n") || (len(trimmed) > 0 && strings.ContainsRune("=+-@", rune(trimmed[0]))) {
				value = "'" + value
			}
			record[i] = value
		}
		if err = writer.Write(record); err != nil {
			return state, err
		}
		state.Processed++
		// Flush both progress and output in bounded batches, without a count query.
		if state.Processed%500 == 0 {
			writer.Flush()
			if err = writer.Error(); err != nil {
				return state, err
			}
			state.Bytes, err = file.Seek(0, io.SeekCurrent)
			if err != nil {
				return state, err
			}
			if err = model.UpdateSystemTaskState(task.TaskID, runnerID, state); err != nil {
				return state, err
			}
		}
	}
	if err = rows.Err(); err != nil {
		return state, err
	}
	writer.Flush()
	if err = writer.Error(); err != nil {
		return state, err
	}
	state.Bytes, err = file.Seek(0, io.SeekCurrent)
	if err != nil {
		return state, err
	}
	if err = file.Sync(); err != nil {
		return state, err
	}
	if err = file.Close(); err != nil {
		return state, err
	}
	if err = os.Rename(path+".part", path); err != nil {
		return state, err
	}
	return state, nil
}

func StartLogExport(payload LogExportPayload) (*model.SystemTask, error) {
	if err := ValidateLogExport(&payload); err != nil {
		return nil, err
	}
	// The existing unique active_key and DB lease enforce one export cluster-wide.
	task, created, err := EnqueueSystemTask(LogExportTaskType, payload)
	if err != nil {
		return nil, err
	}
	if !created {
		return nil, fmt.Errorf("an export is already pending or running; try again later")
	}
	return task, nil
}
