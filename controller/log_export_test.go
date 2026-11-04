package controller

import (
	"bytes"
	"context"
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
		{UserId: 7, CreatedAt: now, Type: 2, ModelName: "kimi-k3.1", Username: "alice", TokenName: "production", Group: "vip", ChannelId: 9, RequestId: "req-a", UpstreamRequestId: "up-a", Quota: 125, Content: "  =1+1,\"quoted\"\nsecond line"},
		{UserId: 8, CreatedAt: now, Type: 2, ModelName: "kimi-k3.1", Username: "bob", TokenName: "production", Group: "vip", ChannelId: 9, RequestId: "req-b", Content: "another user"},
		{UserId: 7, CreatedAt: now, Type: 5, ModelName: "kimi-k3.1", TokenName: "production", Group: "vip", ChannelId: 9, RequestId: "req-error"},
		{UserId: 7, CreatedAt: now - 3600, Type: 2, ModelName: "kimi-k3.1", TokenName: "production", Group: "vip", ChannelId: 9, RequestId: "req-old"},
	}
	require.NoError(t, model.LOG_DB.Create(&rows).Error)
	payload := service.LogExportPayload{OwnerID: 7, Role: common.RoleCommonUser, Self: true,
		Filter: model.LogExportFilter{Type: 2, StartTimestamp: now - 1, EndTimestamp: now + 1, ModelName: "kimi%", TokenName: "production", Group: "vip"},
		Fields: []string{"request_id", "quota", "content"},
	}
	task := runExport(t, payload)
	require.Equal(t, model.SystemTaskStatusSucceeded, task.Status, task.Error)
	data, err := os.ReadFile(service.LogExportFile(task.TaskID))
	require.NoError(t, err)
	records, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\xef\xbb\xbf"))).ReadAll()
	require.NoError(t, err)
	assert.Equal(t, [][]string{{"request_id", "quota", "content"}, {"req-a", "125", "'  =1+1,\"quoted\"\nsecond line"}}, records)
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
	payload.Filter.StartTimestamp = now - 8*86400
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
