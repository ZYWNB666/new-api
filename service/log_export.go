package service

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

const LogExportTaskType = "log_export"
const LogExportRetention = 24 * time.Hour

type LogExportPayload struct {
	OwnerID     int                   `json:"owner_id"`
	Role        int                   `json:"role"`
	Self        bool                  `json:"self"`
	Filter      model.LogExportFilter `json:"filter"`
	Fields      []string              `json:"fields"`
	Mode        string                `json:"mode,omitempty"`
	Granularity string                `json:"granularity,omitempty"`
	Timezone    string                `json:"timezone,omitempty"`
	GroupBy     []string              `json:"group_by,omitempty"`
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
	if payload.Mode == "" {
		payload.Mode = "details"
	}
	if payload.Mode != "details" && payload.Mode != "statistics" {
		return errors.New("invalid export mode")
	}
	if payload.Mode == "statistics" {
		if !slices.Contains([]string{"hour", "day", "month", "all"}, payload.Granularity) {
			return errors.New("invalid summary granularity")
		}
		if payload.Timezone == "" {
			payload.Timezone = "UTC"
		}
		if len(payload.Timezone) > 64 {
			return errors.New("invalid summary timezone")
		}
		if _, err := time.LoadLocation(payload.Timezone); err != nil {
			return errors.New("invalid summary timezone")
		}
		if len(payload.GroupBy) > 6 {
			return errors.New("too many summary dimensions")
		}
		seen := map[string]bool{}
		for _, field := range payload.GroupBy {
			if !slices.Contains([]string{"model_name", "user_id", "channel", "group", "token_name", "token_id"}, field) || seen[field] {
				return errors.New("invalid summary dimension")
			}
			if payload.Self && (field == "user_id" || field == "channel") {
				return errors.New("admin summary dimension is not allowed")
			}
			seen[field] = true
		}
		payload.Fields = logExportSummaryHeaders(*payload)
	} else if _, err := model.LogExportColumns(payload.Fields, payload.Self); err != nil {
		return err
	}
	f := &payload.Filter
	// The log page defaults its end time to now + 1 hour. Freeze the export
	// boundary at creation instead of including future traffic.
	f.EndTimestamp = min(f.EndTimestamp, common.GetTimestamp())
	maxDays := 31
	if payload.Mode == "statistics" {
		maxDays = 366
	}
	if f.StartTimestamp <= 0 || f.EndTimestamp < f.StartTimestamp || f.EndTimestamp-f.StartTimestamp > int64(maxDays)*86400 {
		return fmt.Errorf("export requires a past time range of at most %d days", maxDays)
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
	headers := payload.Fields
	readFields := payload.Fields
	if payload.Mode == "statistics" {
		headers = logExportSummaryHeaders(payload)
		readFields = []string{"created_at", "type", "model_name", "token_name", "group", "prompt_tokens", "completion_tokens", "quota", "use_time", "amount_usd", "token_id"}
		if !payload.Self {
			readFields = append(readFields, "user_id", "channel")
		}
	}
	if err = writer.Write(headers); err != nil {
		return state, err
	}
	rows, err := model.OpenLogExportRows(ctx, payload.Filter, readFields)
	if err != nil {
		return state, err
	}
	defer rows.Close()
	summaries := map[string]*logExportSummary{}
	priceCache := map[logExportPriceKey]map[string]decimal.Decimal{}
	location, _ := time.LoadLocation(payload.Timezone)
	scanned := int64(0)
	for rows.Next() {
		if err = ctx.Err(); err != nil {
			return state, err
		}
		if scanned >= int64(maxRows) {
			return state, errors.New("export row limit exceeded; narrow the filters")
		}
		var log model.Log
		if err = model.LOG_DB.ScanRows(rows, &log); err != nil {
			return state, err
		}
		scanned++
		values := logExportBillingValues(log, priceCache)
		if payload.Mode == "statistics" {
			if err = addLogExportSummary(summaries, payload, location, log, values); err != nil {
				return state, err
			}
			if scanned%500 == 0 {
				if err = model.UpdateSystemTaskState(task.TaskID, runnerID, state); err != nil {
					return state, err
				}
			}
			continue
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
			case "token_id":
				value = strconv.Itoa(log.TokenId)
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
			default:
				value = values[field]
			}
			if strings.HasSuffix(field, "_usd") {
				// Generated decimal amounts may be negative rounding adjustments.
				record[i] = value
			} else {
				record[i] = safeLogExportCell(value)
			}
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
	if payload.Mode == "statistics" {
		keys := make([]string, 0, len(summaries))
		for key := range summaries {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			if err = ctx.Err(); err != nil {
				return state, err
			}
			if err = writer.Write(summaries[key].record()); err != nil {
				return state, err
			}
			state.Processed++
		}
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

// CSV quoting alone does not prevent spreadsheet formula execution.
func safeLogExportCell(value string) string {
	trimmed := strings.TrimLeftFunc(value, unicode.IsSpace)
	if strings.HasPrefix(value, "\t") || strings.HasPrefix(value, "\r") || strings.HasPrefix(value, "\n") || (len(trimmed) > 0 && strings.ContainsRune("=+-@", rune(trimmed[0]))) {
		return "'" + value
	}
	return value
}
