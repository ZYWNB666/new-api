package model

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	"github.com/QuantumNous/new-api/common"
)

// LogExportFilter uses the same filter semantics as the usage-log list. UserID
// is set by the server for self exports, never taken from a client request.
type LogExportFilter struct {
	Type              int    `json:"type"`
	StartTimestamp    int64  `json:"start_timestamp"`
	EndTimestamp      int64  `json:"end_timestamp"`
	ModelName         string `json:"model_name"`
	Username          string `json:"username"`
	TokenName         string `json:"token_name"`
	Channel           int    `json:"channel"`
	Group             string `json:"group"`
	RequestID         string `json:"request_id"`
	UpstreamRequestID string `json:"upstream_request_id"`
	UserID            int    `json:"-"`
}

// OpenLogExportRows holds one read connection and streams one statement's
// result. There is no COUNT, OFFSET, or million-row in-memory slice. A single
// statement also avoids shifting pages when live traffic appends new logs.
func OpenLogExportRows(ctx context.Context, filter LogExportFilter, fields []string) (*sql.Rows, error) {
	columns, err := LogExportColumns(fields, filter.UserID > 0)
	if err != nil {
		return nil, err
	}
	tx := LOG_DB.WithContext(ctx).Model(&Log{}).
		Select(columns).
		Where("logs.created_at >= ? AND logs.created_at <= ?", filter.StartTimestamp, filter.EndTimestamp)
	if filter.UserID > 0 {
		tx = tx.Where("logs.user_id = ?", filter.UserID)
	}
	if filter.Type != LogTypeUnknown {
		tx = tx.Where("logs.type = ?", filter.Type)
	}
	if tx, err = applyExplicitLogTextFilter(tx, "logs.model_name", filter.ModelName); err != nil {
		return nil, err
	}
	if tx, err = applyExplicitLogTextFilter(tx, "logs.username", filter.Username); err != nil {
		return nil, err
	}
	for column, value := range map[string]string{
		"token_name": filter.TokenName, "group": filter.Group,
		"request_id": filter.RequestID, "upstream_request_id": filter.UpstreamRequestID,
	} {
		if value != "" {
			// Map keys are fixed identifiers; GORM quotes reserved names per dialect.
			tx = tx.Where(map[string]any{column: value})
		}
	}
	if filter.Channel != 0 {
		tx = tx.Where("logs.channel_id = ?", filter.Channel)
	}
	order := "logs.created_at desc, logs.id desc"
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		order = clickHouseLogOrder("logs.")
	}
	return tx.Order(order).Rows()
}

// LogExportColumns is shared by request validation and the database projection.
// Full metadata blobs and privileged diagnostics are deliberately not exportable.
func LogExportColumns(fields []string, self bool) ([]string, error) {
	if len(fields) == 0 || len(fields) > 64 {
		return nil, errors.New("select valid export fields")
	}
	columns := make([]string, 0, len(fields))
	seen := make(map[string]bool, len(fields))
	for _, field := range fields {
		if seen[field] {
			return nil, errors.New("duplicate export field")
		}
		seen[field] = true
		switch field {
		case "client_request_id", "billing_source", "billing_unit", "billing_tokens", "billing_prices", "billing_expression", "billing_quota_per_unit", "subscription_consumed",
			"input_tokens_total", "cache_read_tokens", "cache_write_tokens", "cache_write_tokens_5m", "cache_write_tokens_1h", "image_input_tokens", "image_output_tokens", "audio_input_tokens", "audio_output_tokens",
			"billing_input_tokens", "billing_output_tokens", "amount_usd", "input_amount_usd", "output_amount_usd", "cache_read_amount_usd", "cache_write_amount_usd", "image_input_amount_usd", "image_output_amount_usd", "audio_input_amount_usd", "audio_output_amount_usd", "other_amount_usd":
			columns = append(columns, "other", "quota", "prompt_tokens", "completion_tokens")
		case "created_at", "request_id", "upstream_request_id", "type", "model_name", "token_name", "token_id", "prompt_tokens", "completion_tokens", "quota", "use_time", "is_stream", "group", "ip", "content":
			columns = append(columns, field)
		case "user_id", "username", "channel":
			if self {
				return nil, errors.New("admin export field is not allowed")
			}
			if field == "channel" {
				field = "channel_id"
			}
			columns = append(columns, field)
		default:
			return nil, errors.New("invalid export field")
		}
	}
	slices.Sort(columns)
	return slices.Compact(columns), nil
}
