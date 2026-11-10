package controller

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/csv"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func logExportDB(t *testing.T) *gorm.DB {
	t.Helper()
	var dialect gorm.Dialector = sqlite.Open(filepath.Join(t.TempDir(), "export.db") + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(30000)")
	dbType := common.DatabaseTypeSQLite
	if dsn := os.Getenv("LOG_EXPORT_TEST_MYSQL"); dsn != "" {
		dialect = mysql.Open(dsn)
		dbType = common.DatabaseTypeMySQL
	}
	if dsn := os.Getenv("LOG_EXPORT_TEST_POSTGRES"); dsn != "" {
		dialect = postgres.Open(dsn)
		dbType = common.DatabaseTypePostgreSQL
	}
	db, err := gorm.Open(dialect, &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(5)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	mainType, logType := common.MainDatabaseType(), common.LogDatabaseType()
	model.DB, model.LOG_DB = db, db
	common.SetDatabaseTypes(dbType, dbType)
	var version string
	versionQuery := "SELECT version()"
	if dbType == common.DatabaseTypeSQLite {
		versionQuery = "SELECT sqlite_version()"
	}
	require.NoError(t, db.Raw(versionQuery).Scan(&version).Error)
	t.Logf("database=%s version=%s", dbType, version)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(mainType, logType)
		require.NoError(t, sqlDB.Close())
	})
	// Only the existing tables are used; no new schema is introduced.
	require.NoError(t, db.Migrator().DropTable(&model.Log{}, &model.SystemTaskLock{}, &model.SystemTask{}))
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.SystemTask{}, &model.SystemTaskLock{}))
	if dsn := os.Getenv("LOG_EXPORT_TEST_CLICKHOUSE"); dsn != "" {
		t.Setenv("LOG_SQL_DSN", dsn)
		wasMaster := common.IsMasterNode
		common.IsMasterNode = true
		t.Cleanup(func() { common.IsMasterNode = wasMaster })
		require.NoError(t, model.InitLogDB())
		logSQL, err := model.LOG_DB.DB()
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, logSQL.Close()) })
		require.NoError(t, model.LOG_DB.Exec("TRUNCATE TABLE logs").Error)
	}
	t.Setenv("LOG_EXPORT_DIR", t.TempDir())
	return db
}

func exportRequest(t *testing.T, method, path string, body any, owner, role int, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	data, err := common.Marshal(body)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	router := gin.New()
	router.Handle(method, path, func(c *gin.Context) { c.Set("id", owner); c.Set("role", role); c.Next() }, handler)
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, req)
	return recorder
}

func runExport(t *testing.T, payload service.LogExportPayload) *model.SystemTask {
	t.Helper()
	task, err := service.StartLogExport(payload)
	require.NoError(t, err)
	task, claimed, err := model.ClaimSystemTask(task.ID, service.LogExportTaskType, "export-test", time.Now().Unix()+120)
	require.NoError(t, err)
	require.True(t, claimed)
	service.LogExportHandler{}.Run(context.Background(), task, "export-test")
	task, err = model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	return task
}

func TestLogExportFiltersFieldsAndPermissions(t *testing.T) {
	db := logExportDB(t)
	now := time.Now().Unix() - 60
	rows := []model.Log{
		{UserId: 7, CreatedAt: now, Type: 2, ModelName: "kimi-k3.1", Username: "alice", TokenName: "production", Group: "vip", ChannelId: 9, RequestId: "req-a", UpstreamRequestId: "up-a", Other: `{"client_request_id":"customer-a","admin_info":{"diagnostic":"never-export"}}`, Quota: 125, Content: "  =1+1,\"quoted\"\nsecond line"},
		{UserId: 8, CreatedAt: now, Type: 2, ModelName: "kimi-k3.1", Username: "bob", TokenName: "production", Group: "vip", ChannelId: 9, RequestId: "req-b", Content: "another user"},
		{UserId: 7, CreatedAt: now, Type: 5, ModelName: "kimi-k3.1", TokenName: "production", Group: "vip", ChannelId: 9, RequestId: "req-error"},
		{UserId: 7, CreatedAt: now - 3600, Type: 2, ModelName: "kimi-k3.1", TokenName: "production", Group: "vip", ChannelId: 9, RequestId: "req-old"},
	}
	require.NoError(t, model.LOG_DB.Create(&rows).Error)
	payload := service.LogExportPayload{OwnerID: 7, Role: common.RoleCommonUser, Self: true,
		Filter: model.LogExportFilter{Type: 2, StartTimestamp: now - 1, EndTimestamp: now + 1, ModelName: "kimi%", TokenName: "production", Group: "vip"},
		Fields: []string{"request_id", "quota", "content", "client_request_id"},
	}
	task := runExport(t, payload)
	require.Equal(t, model.SystemTaskStatusSucceeded, task.Status, task.Error)
	data, err := os.ReadFile(service.LogExportFile(task.TaskID))
	require.NoError(t, err)
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\xef\xbb\xbf"))).ReadAll()
	require.NoError(t, err)
	assert.Equal(t, [][]string{{"request_id", "quota", "content", "client_request_id"}, {"req-a", "125", "'  =1+1,\"quoted\"\nsecond line", "customer-a"}}, records)
	var state service.LogExportState
	require.NoError(t, common.UnmarshalJsonStr(task.Result, &state))
	assert.Equal(t, int64(1), state.Processed)
	assert.Equal(t, int64(len(data)), state.Bytes)

	for _, tc := range []struct {
		name              string
		owner, role, want int
	}{
		{"owner", 7, common.RoleCommonUser, 200}, {"another owner", 8, common.RoleRootUser, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := "/" + task.TaskID
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, path, nil)
			c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
			c.Set("id", tc.owner)
			c.Set("role", tc.role)
			DownloadLogExport(c)
			assert.Equal(t, tc.want, recorder.Code)
			if tc.want == 200 {
				assert.Equal(t, data, recorder.Body.Bytes())
				assert.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
			}
		})
	}
	// Admin filters are honored together, without exposing arbitrary metadata.
	payload.Self = false
	payload.Role = common.RoleAdminUser
	payload.Filter.Username = "ali%"
	payload.Filter.Channel = 9
	payload.Filter.RequestID = "req-a"
	payload.Filter.UpstreamRequestID = "up-a"
	payload.Fields = []string{"username", "channel", "request_id"}
	adminTask := runExport(t, payload)
	require.Equal(t, model.SystemTaskStatusSucceeded, adminTask.Status, adminTask.Error)
	data, err = os.ReadFile(service.LogExportFile(adminTask.TaskID))
	require.NoError(t, err)
	assert.Contains(t, string(data), "alice,9,req-a")
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Params = gin.Params{{Key: "task_id", Value: adminTask.TaskID}}
	c.Set("id", 7)
	c.Set("role", common.RoleCommonUser)
	DownloadLogExport(c)
	assert.Equal(t, 404, rec.Code, "demoted creator cannot download all-user data")
	require.NoError(t, db.Model(&model.SystemTask{}).Where("task_id = ?", task.TaskID).Update("created_at", time.Now().Add(-25*time.Hour).Unix()).Error)
	rec = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	c.Set("id", 7)
	c.Set("role", common.RoleCommonUser)
	DownloadLogExport(c)
	assert.Equal(t, 410, rec.Code)
}

func TestLogExportValidationFailureAndHistory(t *testing.T) {
	db := logExportDB(t)
	now := time.Now().Unix() - 30
	base := service.LogExportPayload{OwnerID: 7, Role: common.RoleCommonUser, Self: true, Filter: model.LogExportFilter{StartTimestamp: now - 1, EndTimestamp: now + 1}, Fields: []string{"request_id"}}
	for _, fields := range [][]string{nil, {"other"}, {"username"}, {"request_id", "request_id"}, {"request_id; DROP TABLE logs"}} {
		payload := base
		payload.Fields = fields
		assert.Error(t, service.ValidateLogExport(&payload))
	}
	payload := base
	payload.Filter.StartTimestamp = now - 32*86400
	assert.Error(t, service.ValidateLogExport(&payload))
	payload = base
	payload.Filter.EndTimestamp = now - 2
	assert.Error(t, service.ValidateLogExport(&payload))
	recorder := exportRequest(t, "POST", "/export", map[string]any{"scope": "all", "fields": []string{"request_id"}}, 7, common.RoleCommonUser, CreateLogExport)
	assert.Equal(t, 403, recorder.Code)
	recorder = exportRequest(t, "POST", "/export", map[string]any{"scope": "self", "filter": base.Filter, "fields": []string{"other"}}, 7, common.RoleCommonUser, CreateLogExport)
	assert.Equal(t, 400, recorder.Code)
	require.NoError(t, model.LOG_DB.Create(&[]model.Log{{UserId: 7, CreatedAt: now, RequestId: "first"}, {UserId: 7, CreatedAt: now, RequestId: "second"}}).Error)
	t.Setenv("LOG_EXPORT_MAX_ROWS", "1")
	task := runExport(t, base)
	require.Equal(t, model.SystemTaskStatusFailed, task.Status)
	_, err := os.Stat(service.LogExportFile(task.TaskID))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(service.LogExportFile(task.TaskID) + ".part")
	assert.True(t, os.IsNotExist(err))
	recorder = exportRequest(t, "GET", "/export", nil, 8, common.RoleRootUser, ListLogExports)
	assert.NotContains(t, recorder.Body.String(), task.TaskID)
	recorder = exportRequest(t, "GET", "/export", nil, 7, common.RoleCommonUser, ListLogExports)
	assert.Contains(t, recorder.Body.String(), task.TaskID)
	assert.NotContains(t, recorder.Body.String(), task.Error)
	pending, err := service.StartLogExport(base)
	require.NoError(t, err)
	_, err = service.StartLogExport(base)
	assert.Error(t, err)
	var count int64
	require.NoError(t, db.Model(&model.SystemTask{}).Where("status = ?", model.SystemTaskStatusPending).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	assert.NotEmpty(t, pending.TaskID)
}

func TestLogExportBatchProgressAndEmptyResult(t *testing.T) {
	logExportDB(t)
	now := time.Now().Unix() - 60
	// Cross exactly one progress-flush boundary. This protects streaming reads
	// alongside task writes when main and log databases share one pool.
	logs := make([]model.Log, 501)
	for i := range logs {
		logs[i] = model.Log{UserId: 7, CreatedAt: now, RequestId: "batch-" + strconv.Itoa(i), Group: "vip", IsStream: true}
	}
	require.NoError(t, model.LOG_DB.Create(&logs).Error)
	payload := service.LogExportPayload{OwnerID: 7, Role: common.RoleCommonUser, Self: true,
		Filter: model.LogExportFilter{StartTimestamp: now - 1, EndTimestamp: now + 1, Group: "vip"},
		Fields: []string{"request_id", "group", "is_stream"},
	}
	task := runExport(t, payload)
	require.Equal(t, model.SystemTaskStatusSucceeded, task.Status, task.Error)
	var progress service.LogExportState
	require.NoError(t, task.DecodeState(&progress))
	assert.Equal(t, int64(500), progress.Processed)
	data, err := os.ReadFile(service.LogExportFile(task.TaskID))
	require.NoError(t, err)
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\xef\xbb\xbf"))).ReadAll()
	require.NoError(t, err)
	assert.Len(t, records, 502)
	assert.Equal(t, []string{"vip", "true"}, records[1][1:])
	payload.Filter.RequestID = "missing-request"
	task = runExport(t, payload)
	require.Equal(t, model.SystemTaskStatusSucceeded, task.Status, task.Error)
	data, err = os.ReadFile(service.LogExportFile(task.TaskID))
	require.NoError(t, err)
	assert.Equal(t, "\xef\xbb\xbfrequest_id,group,is_stream\r\n", string(data))
	// A single oversized cell must fail without waiting for 500 records, and
	// must never publish a partial file as a successful export.
	t.Setenv("LOG_EXPORT_MAX_MB", "1")
	require.NoError(t, model.LOG_DB.Create(&model.Log{UserId: 7, CreatedAt: now, RequestId: "oversized", Content: strings.Repeat("x", 2<<20)}).Error)
	payload.Filter.Group = ""
	payload.Filter.RequestID = "oversized"
	payload.Fields = []string{"content"}
	task = runExport(t, payload)
	require.Equal(t, model.SystemTaskStatusFailed, task.Status)
	_, err = os.Stat(service.LogExportFile(task.TaskID))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(service.LogExportFile(task.TaskID) + ".part")
	assert.True(t, os.IsNotExist(err))
}

func TestLogExportBillingDetailsAndStatistics(t *testing.T) {
	logExportDB(t)
	now := time.Now().UTC().Truncate(time.Hour).Add(-2 * time.Hour).Unix()
	rows := []model.Log{
		{UserId: 7, CreatedAt: now + 60, Type: 2, ModelName: "model-a", Group: "vip", ChannelId: 9, PromptTokens: 1000, CompletionTokens: 100, Quota: 2000, Other: `{"billing_quota_per_unit":500000,"billing_charge_tokens":{"p":700,"c":100,"cr":200,"cc":100},"billing_prices":{"p":2,"c":8,"cr":0.2,"cc":2.5},"cache_tokens":200,"cache_write_tokens":100,"admin_info":{"secret":"never-export"}}`},
		{UserId: 7, CreatedAt: now + 120, Type: 2, ModelName: "model-a", Group: "vip", ChannelId: 9, PromptTokens: 1000, CompletionTokens: 100, Quota: 2000, Other: `{"billing_quota_per_unit":500000,"billing_charge_tokens":{"p":700,"c":100,"cr":200,"cc":100},"billing_prices":{"p":2,"c":8,"cr":0.2,"cc":2.5},"cache_tokens":200}`},
		{UserId: 7, CreatedAt: now + 3660, Type: 2, ModelName: "model-a", Quota: 100, Other: `{}`},
		{UserId: 8, CreatedAt: now + 60, Type: 2, ModelName: "model-a", Quota: 99999},
	}
	require.NoError(t, model.LOG_DB.Create(&rows).Error)
	payload := service.LogExportPayload{OwnerID: 7, Role: common.RoleCommonUser, Self: true,
		Filter: model.LogExportFilter{StartTimestamp: now, EndTimestamp: now + 7200},
		Fields: []string{"created_at", "cache_read_tokens", "cache_write_tokens", "billing_input_tokens", "amount_usd", "input_amount_usd", "output_amount_usd", "cache_read_amount_usd", "cache_write_amount_usd", "other_amount_usd"},
	}
	task := runExport(t, payload)
	require.Equal(t, model.SystemTaskStatusSucceeded, task.Status, task.Error)
	data, err := os.ReadFile(service.LogExportFile(task.TaskID))
	require.NoError(t, err)
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\xef\xbb\xbf"))).ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 4)
	assert.Equal(t, []string{"200", "100", "700", "0.004", "0.0014", "0.0008", "0.00004", "0.00025", "0.00151"}, records[2][1:])
	assert.Equal(t, "", records[1][4], "missing historical conversion must remain unknown")
	assert.NotContains(t, string(data), "never-export")

	payload.Mode = "statistics"
	payload.Granularity = "hour"
	payload.Timezone = "UTC"
	payload.GroupBy = []string{"model_name"}
	task = runExport(t, payload)
	require.Equal(t, model.SystemTaskStatusSucceeded, task.Status, task.Error)
	data, err = os.ReadFile(service.LogExportFile(task.TaskID))
	require.NoError(t, err)
	records, err = csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\xef\xbb\xbf"))).ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 3)
	indexed := map[string]string{}
	for i, key := range records[0] {
		indexed[key] = records[1][i]
	}
	assert.Equal(t, "2", indexed["log_count"])
	assert.Equal(t, "2000", indexed["prompt_tokens"])
	assert.Equal(t, "400", indexed["cache_read_tokens"])
	assert.Equal(t, "0.008", indexed["amount_usd"])
	assert.Equal(t, "0", indexed["amount_missing_logs"])
	for i, key := range records[0] {
		indexed[key] = records[2][i]
	}
	assert.Equal(t, "", indexed["amount_usd"])
	assert.Equal(t, "1", indexed["amount_missing_logs"])
	payload.GroupBy = []string{"channel"}
	assert.Error(t, service.ValidateLogExport(&payload), "self export cannot group by privileged dimensions")
	payload.GroupBy = nil
	payload.Granularity = "invalid"
	assert.Error(t, service.ValidateLogExport(&payload))
}

func TestLogExportFrozenExpressionAndSubscriptionAmounts(t *testing.T) {
	logExportDB(t)
	now := time.Now().Unix() - 60
	expression := `(len <= 100 ? tier("short", p * 2 + c * 4 + cr * 0.5 + cc * 3) : tier("long", p * 4 + c * 8 + cr * 1 + cc * 6)) * (header("x-fast") == "yes" ? 2 : 1)`
	for _, tc := range []struct {
		name, expr                       string
		tokens                           map[string]float64
		source                           string
		consumed                         int
		wantInput, wantOutput, wantTotal string
	}{
		{"linear tier and frozen multiplier", expression, map[string]float64{"p": 200, "c": 10, "cr": 50, "cc": 20}, "wallet", 0, "0.0012", "0.00012", "0.004"},
		{"subscription uses consumed quota", `tier("long",p * 4 + c * 8)`, map[string]float64{"p": 200, "c": 10}, "subscription", 500, "0.0006", "0.00006", "0.001"},
		{"nonlinear expression preserves unknown components", `tier("long",p * c)`, map[string]float64{"p": 200, "c": 10}, "wallet", 0, "", "", "0.004"},
		{"fixed request preserves unknown token amounts", `tier("long",fixed(0.004))`, nil, "wallet", 0, "", "", "0.004"},
		{"untraced request condition must not select a guessable branch", `tier("long",p * 4) * (hour("UTC") > 12 ? 2 : max(p, 1))`, map[string]float64{"p": 200, "c": 10}, "wallet", 0, "", "", "0.004"},
		{"missing tier trace must not guess the historical branch", `hour("UTC") > 12 ? p * 4 : max(p, 1)`, map[string]float64{"p": 200, "c": 10}, "wallet", 0, "", "", "0.004"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			other, err := common.Marshal(map[string]any{"billing_mode": "tiered_expr", "expr_b64": base64.StdEncoding.EncodeToString([]byte(tc.expr)), "matched_tier": "long", "billing_tokens": tc.tokens, "billing_group_ratio": 0.75, "billing_quota_per_unit": 500000, "billing_source": tc.source, "subscription_consumed": tc.consumed, "request_rules": []map[string]any{{"cond": `header("x-fast") == "yes"`, "matched": true, "multiplier": 2}}})
			require.NoError(t, err)
			row := model.Log{UserId: 7, Type: 2, CreatedAt: now, RequestId: tc.name, Quota: 2000, Other: string(other)}
			require.NoError(t, model.LOG_DB.Create(&row).Error)
			payload := service.LogExportPayload{OwnerID: 7, Role: common.RoleCommonUser, Self: true, Filter: model.LogExportFilter{StartTimestamp: now - 1, EndTimestamp: now + 1, RequestID: tc.name}, Fields: []string{"input_amount_usd", "output_amount_usd", "amount_usd", "billing_expression"}}
			task := runExport(t, payload)
			require.Equal(t, model.SystemTaskStatusSucceeded, task.Status, task.Error)
			data, err := os.ReadFile(service.LogExportFile(task.TaskID))
			require.NoError(t, err)
			records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\xef\xbb\xbf"))).ReadAll()
			require.NoError(t, err)
			require.Len(t, records, 2)
			assert.Equal(t, []string{tc.wantInput, tc.wantOutput, tc.wantTotal, tc.expr}, records[1])
		})
	}
}

func TestLogExportSummaryGrainsDimensionsAndUnknownTotals(t *testing.T) {
	logExportDB(t)
	now := time.Date(2026, 10, 31, 15, 30, 0, 0, time.UTC).Unix()
	rows := []model.Log{
		{UserId: 7, Type: 2, CreatedAt: now, ModelName: "=spreadsheet", Group: "vip", ChannelId: 9, TokenId: 1, Quota: 500000, Other: `{"billing_source":"wallet","billing_quota_per_unit":500000}`},
		{UserId: 7, Type: 2, CreatedAt: now + 3600, ModelName: "=spreadsheet", Group: "vip", ChannelId: 9, TokenId: 2, Quota: 500000, Other: `{"billing_source":"wallet","billing_quota_per_unit":500000}`},
		{UserId: 7, Type: 2, CreatedAt: now + 3660, ModelName: "=spreadsheet", Group: "vip", ChannelId: 9, TokenId: 2, Quota: 100, Other: `{"billing_source":"wallet"}`},
		{UserId: 7, Type: 6, CreatedAt: now, ModelName: "=spreadsheet", Quota: 500000, Other: `{"billing_source":"wallet","billing_quota_per_unit":500000}`},
		{UserId: 7, Type: 2, CreatedAt: now, ModelName: "=spreadsheet", Quota: 0, Other: `{"billing_source":"subscription","subscription_consumed":500000,"billing_quota_per_unit":500000}`},
	}
	require.NoError(t, model.LOG_DB.Create(&rows).Error)
	for _, tc := range []struct {
		grain      string
		dimensions []string
		wantRows   int
	}{
		{"hour", nil, 4}, {"day", nil, 4}, {"month", nil, 4}, {"all", nil, 3}, {"all", []string{"user_id", "channel", "group", "token_id"}, 4},
	} {
		t.Run(tc.grain+strings.Join(tc.dimensions, "-"), func(t *testing.T) {
			payload := service.LogExportPayload{OwnerID: 7, Role: common.RoleAdminUser, Mode: "statistics", Granularity: tc.grain, Timezone: "Asia/Shanghai", GroupBy: tc.dimensions, Filter: model.LogExportFilter{StartTimestamp: now - 1, EndTimestamp: now + 7200}}
			task := runExport(t, payload)
			require.Equal(t, model.SystemTaskStatusSucceeded, task.Status, task.Error)
			data, err := os.ReadFile(service.LogExportFile(task.TaskID))
			require.NoError(t, err)
			records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\xef\xbb\xbf"))).ReadAll()
			require.NoError(t, err)
			assert.Len(t, records, tc.wantRows+1)
			if tc.grain == "day" {
				assert.Contains(t, string(data), "2026-11-01T00:00:00+08:00")
			}
			if tc.grain == "month" {
				assert.Contains(t, string(data), "2026-11-01T00:00:00+08:00")
			}
			if tc.grain == "all" && len(tc.dimensions) == 0 {
				for _, record := range records[1:] {
					if record[1] == "2" && record[2] == "wallet" {
						indexed := map[string]string{}
						for i, key := range records[0] {
							indexed[key] = record[i]
						}
						assert.Equal(t, "3", indexed["log_count"])
						assert.Equal(t, "1", indexed["amount_missing_logs"])
						assert.Empty(t, indexed["amount_usd"])
					}
				}
			}
		})
	}
	payload := service.LogExportPayload{OwnerID: 7, Role: common.RoleAdminUser, Mode: "statistics", Granularity: "all", GroupBy: []string{"model_name"}, Filter: model.LogExportFilter{StartTimestamp: now - 1, EndTimestamp: now + 7200}}
	task := runExport(t, payload)
	require.Equal(t, model.SystemTaskStatusSucceeded, task.Status, task.Error)
	data, err := os.ReadFile(service.LogExportFile(task.TaskID))
	require.NoError(t, err)
	assert.Contains(t, string(data), "'=spreadsheet")
	payload.Granularity = "month"
	payload.Filter.StartTimestamp = now - 30*86400
	assert.NoError(t, service.ValidateLogExport(&payload), "statistics supports a full month")
	payload.Filter.StartTimestamp = now - 367*86400
	assert.Error(t, service.ValidateLogExport(&payload))
	payload.Filter.StartTimestamp = now - 1
	for _, tc := range []struct {
		mode, grain, zone string
		dims              []string
	}{
		{"invalid", "all", "UTC", nil}, {"statistics", "all", "invalid/timezone", nil}, {"statistics", "all", "UTC", []string{"user_id", "user_id"}}, {"statistics", "all", "UTC", []string{"other"}},
	} {
		payload.Mode, payload.Granularity, payload.Timezone, payload.GroupBy = tc.mode, tc.grain, tc.zone, tc.dims
		assert.Error(t, service.ValidateLogExport(&payload))
	}
}

func TestLogExportSummaryDSTAndRowLimit(t *testing.T) {
	logExportDB(t)
	first := time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC).Unix()
	require.NoError(t, model.LOG_DB.Create(&[]model.Log{{UserId: 7, Type: 2, CreatedAt: first}, {UserId: 7, Type: 2, CreatedAt: first + 3600}}).Error)
	payload := service.LogExportPayload{OwnerID: 7, Role: common.RoleCommonUser, Self: true, Mode: "statistics", Granularity: "hour", Timezone: "America/New_York", Filter: model.LogExportFilter{StartTimestamp: first - 60, EndTimestamp: first + 7200}}
	task := runExport(t, payload)
	require.Equal(t, model.SystemTaskStatusSucceeded, task.Status, task.Error)
	data, err := os.ReadFile(service.LogExportFile(task.TaskID))
	require.NoError(t, err)
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\xef\xbb\xbf"))).ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 3)
	assert.Equal(t, "2026-11-01T01:00:00-04:00", records[1][0])
	assert.Equal(t, "2026-11-01T01:00:00-05:00", records[2][0])
	t.Setenv("LOG_EXPORT_MAX_ROWS", "1")
	task = runExport(t, payload)
	require.Equal(t, model.SystemTaskStatusFailed, task.Status)
	_, err = os.Stat(service.LogExportFile(task.TaskID))
	assert.True(t, os.IsNotExist(err))
	payload.Filter.RequestID = "no-matching-record"
	task = runExport(t, payload)
	require.Equal(t, model.SystemTaskStatusSucceeded, task.Status, task.Error)
	data, err = os.ReadFile(service.LogExportFile(task.TaskID))
	require.NoError(t, err)
	records, err = csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\xef\xbb\xbf"))).ReadAll()
	require.NoError(t, err)
	assert.Len(t, records, 1, "empty summary retains its headers")
}

func TestLogExportDetailMonthRange(t *testing.T) {
	end := time.Date(2026, 11, 1, 0, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600)).Unix()
	for _, tc := range []struct {
		name      string
		duration  int64
		wantError bool
	}{
		{"28 day month", 28 * 86400, false},
		{"29 day leap month", 29 * 86400, false},
		{"30 day month", 30 * 86400, false},
		{"31 day month", 31 * 86400, false},
		{"beyond detail range", 31*86400 + 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := service.LogExportPayload{OwnerID: 7, Role: common.RoleCommonUser, Self: true, Mode: "details", Fields: []string{"created_at"}, Filter: model.LogExportFilter{StartTimestamp: end - tc.duration, EndTimestamp: end}}
			err := service.ValidateLogExport(&payload)
			if tc.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, end-tc.duration, payload.Filter.StartTimestamp)
			assert.Equal(t, end, payload.Filter.EndTimestamp)
		})
	}
}
