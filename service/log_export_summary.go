package service

import (
	"errors"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
)

// Bound memory independently of the input row limit. High-cardinality summaries
// fail explicitly rather than truncate groups or accumulate millions of entries.
const maxLogExportGroups = 10000

type logExportSummary struct {
	dimensions []string
	count      int64
	sums       []decimal.Decimal
	missing    []int64
}

func logExportSummaryHeaders(payload LogExportPayload) []string {
	headers := []string{"time_bucket", "type", "billing_source"}
	headers = append(headers, payload.GroupBy...)
	headers = append(headers, "log_count", "amount_missing_logs")
	return append(headers, logExportSummaryMetrics...)
}

func addLogExportSummary(groups map[string]*logExportSummary, payload LogExportPayload, location *time.Location, log model.Log, values map[string]string) error {
	bucket := "all"
	if payload.Granularity != "all" {
		at := time.Unix(log.CreatedAt, 0).In(location)
		year, month, day := at.Date()
		hour := at.Hour()
		if payload.Granularity == "day" {
			hour = 0
		}
		if payload.Granularity == "month" {
			hour = 0
			day = 1
		}
		start := time.Date(year, month, day, hour, 0, 0, 0, location)
		if payload.Granularity == "hour" {
			start = at.Add(-time.Duration(at.Minute())*time.Minute - time.Duration(at.Second())*time.Second - time.Duration(at.Nanosecond()))
		}
		bucket = start.Format(time.RFC3339)
	}
	dimensions := []string{bucket, strconv.Itoa(log.Type), values["billing_source"]}
	for _, field := range payload.GroupBy {
		value := ""
		switch field {
		case "model_name":
			value = log.ModelName
		case "user_id":
			value = strconv.Itoa(log.UserId)
		case "channel":
			value = strconv.Itoa(log.ChannelId)
		case "group":
			value = log.Group
		case "token_name":
			value = log.TokenName
		case "token_id":
			value = strconv.Itoa(log.TokenId)
		}
		dimensions = append(dimensions, value)
	}
	encoded, err := common.Marshal(dimensions)
	if err != nil {
		return err
	}
	key := string(encoded)
	group := groups[key]
	if group == nil {
		if len(groups) >= maxLogExportGroups {
			return errors.New("summary group limit exceeded; narrow the filters or use fewer dimensions")
		}
		group = &logExportSummary{dimensions: dimensions, sums: make([]decimal.Decimal, len(logExportSummaryMetrics)), missing: make([]int64, len(logExportSummaryMetrics))}
		groups[key] = group
	}
	group.count++
	values["prompt_tokens"] = strconv.Itoa(log.PromptTokens)
	values["completion_tokens"] = strconv.Itoa(log.CompletionTokens)
	values["quota"] = strconv.Itoa(log.Quota)
	values["use_time"] = strconv.Itoa(log.UseTime)
	for i, field := range logExportSummaryMetrics {
		value, err := decimal.NewFromString(values[field])
		if err != nil {
			group.missing[i]++
			continue
		}
		group.sums[i] = group.sums[i].Add(value)
	}
	return nil
}

func (s *logExportSummary) record() []string {
	record := make([]string, 0, len(s.dimensions)+2+len(s.sums))
	for _, value := range s.dimensions {
		record = append(record, safeLogExportCell(value))
	}
	record = append(record, strconv.FormatInt(s.count, 10))
	missingAmount := int64(0)
	for i, field := range logExportSummaryMetrics {
		if field == "amount_usd" {
			missingAmount = s.missing[i]
		}
	}
	record = append(record, strconv.FormatInt(missingAmount, 10))
	for i, value := range s.sums {
		cell := ""
		if s.missing[i] == 0 {
			cell = value.String()
		}
		// These values are computed numeric cells; negative refunds and rounding
		// adjustments are valid numbers, never untrusted spreadsheet expressions.
		record = append(record, cell)
	}
	return record
}

// Numeric summary fields are additive quantities, never unit prices or ratios.
var logExportSummaryMetrics = []string{"prompt_tokens", "completion_tokens", "quota", "use_time",
	"input_tokens_total", "cache_read_tokens", "cache_write_tokens", "cache_write_tokens_5m", "cache_write_tokens_1h",
	"image_input_tokens", "image_output_tokens", "audio_input_tokens", "audio_output_tokens",
	"billing_input_tokens", "billing_output_tokens", "amount_usd", "input_amount_usd", "output_amount_usd", "cache_read_amount_usd", "cache_write_amount_usd", "image_input_amount_usd", "image_output_amount_usd", "audio_input_amount_usd", "audio_output_amount_usd", "other_amount_usd"}
