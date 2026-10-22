package controller

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testAlipayKeyPair 生成一对 RSA2 密钥供测试使用。
func testAlipayKeyPair(t *testing.T) (privatePEM string, publicPEM string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	privDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})

	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

	return string(privPEM), string(pubPEM)
}

// withAlipayTestClient 用测试密钥构造 alipayClient，测试结束后还原配置。
func withAlipayTestClient(t *testing.T) *alipayClient {
	t.Helper()
	origAppId := setting.AlipayAppId
	origPriv := setting.AlipayAppPrivateKey
	origPub := setting.AlipayPublicKey
	t.Cleanup(func() {
		setting.AlipayAppId = origAppId
		setting.AlipayAppPrivateKey = origPriv
		setting.AlipayPublicKey = origPub
	})

	privPEM, pubPEM := testAlipayKeyPair(t)
	setting.AlipayAppId = "2021000000000000"
	setting.AlipayAppPrivateKey = privPEM
	setting.AlipayPublicKey = pubPEM

	client, err := newAlipayClient()
	require.NoError(t, err)
	return client
}

// TestAlipaySignAndVerifyRoundTrip 验证签名能被同一密钥对验签通过，
// 这是整个回调安全的基础。
func TestAlipaySignAndVerifyRoundTrip(t *testing.T) {
	client := withAlipayTestClient(t)

	params := map[string]string{
		"app_id":      "2021000000000000",
		"method":      "alipay.trade.page.pay",
		"charset":     "utf-8",
		"sign_type":   "RSA2",
		"timestamp":   "2026-10-22 12:00:00",
		"version":     "1.0",
		"biz_content": `{"out_trade_no":"alipay-1-2-abc","total_amount":"10.00"}`,
		"empty_field": "",
	}

	sign, err := client.sign(params)
	require.NoError(t, err)
	require.NotEmpty(t, sign)

	// sign 字段本身不参与待验串
	require.NoError(t, client.verify(`app_id=2021000000000000&biz_content={"out_trade_no":"alipay-1-2-abc","total_amount":"10.00"}&charset=utf-8&method=alipay.trade.page.pay&sign_type=RSA2&timestamp=2026-10-22 12:00:00&version=1.0`, sign))
}

// TestAlipayVerifyRejectsTamperedContent 确保篡改金额后验签失败。
func TestAlipayVerifyRejectsTamperedContent(t *testing.T) {
	client := withAlipayTestClient(t)

	params := map[string]string{
		"app_id":       "2021000000000000",
		"out_trade_no": "alipay-1-2-abc",
		"total_amount": "10.00",
	}
	sign, err := client.sign(params)
	require.NoError(t, err)

	tampered := alipaySignContent(map[string]string{
		"app_id":       "2021000000000000",
		"out_trade_no": "alipay-1-2-abc",
		"total_amount": "999.00", // 攻击者改大金额
	})
	assert.Error(t, client.verify(tampered, sign))
}

// TestAlipayVerifyRejectsForeignSignature 确保其它密钥签发的签名无法通过。
func TestAlipayVerifyRejectsForeignSignature(t *testing.T) {
	client := withAlipayTestClient(t)

	// 用另一对密钥签名
	foreignPriv, _ := testAlipayKeyPair(t)
	foreignClient, err := newAlipayClientFromKeys("2021000000000000", foreignPriv, client.alipayPubKey)
	require.NoError(t, err)

	params := map[string]string{"app_id": "2021000000000000", "total_amount": "10.00"}
	forged, err := foreignClient.sign(params)
	require.NoError(t, err)

	assert.Error(t, client.verify(alipaySignContent(params), forged))
}

// TestAlipaySignContentOrderingAndFiltering 验证待签串的排序与空值过滤规则。
func TestAlipaySignContentOrderingAndFiltering(t *testing.T) {
	got := alipaySignContent(map[string]string{
		"sign":      "should_be_excluded",
		"sign_type": "RSA2",
		"b":         "2",
		"a":         "1",
		"empty":     "",
	})
	// 按 key 升序、跳过 sign/sign_type/空值
	assert.Equal(t, "a=1&b=2&sign_type=RSA2", got)
}

// TestAlipayParsePrivateKeyAcceptsBareBase64 验证开放平台导出的裸 base64 私钥可解析。
func TestAlipayParsePrivateKeyAcceptsBareBase64(t *testing.T) {
	privPEM, _ := testAlipayKeyPair(t)
	block, _ := pem.Decode([]byte(privPEM))
	require.NotNil(t, block)

	bare := base64.StdEncoding.EncodeToString(block.Bytes)
	key, err := parseRSAPrivateKey(bare)
	require.NoError(t, err)
	assert.Equal(t, 2048, key.N.BitLen())
}

// TestAlipayParsePublicKeyAcceptsBareBase64 验证裸 base64 支付宝公钥可解析。
func TestAlipayParsePublicKeyAcceptsBareBase64(t *testing.T) {
	_, pubPEM := testAlipayKeyPair(t)
	block, _ := pem.Decode([]byte(pubPEM))
	require.NotNil(t, block)

	bare := base64.StdEncoding.EncodeToString(block.Bytes)
	key, err := parseRSAPublicKey(bare)
	require.NoError(t, err)
	assert.Equal(t, 2048, key.N.BitLen())
}

// TestAlipayParseRejectsEmptyAndGarbage 确保空值与垃圾输入返回错误而非 panic。
func TestAlipayParseRejectsEmptyAndGarbage(t *testing.T) {
	_, err := parseRSAPrivateKey("")
	assert.Error(t, err)

	_, err = parseRSAPrivateKey("not-a-key-at-all!!!")
	assert.Error(t, err)

	_, err = parseRSAPublicKey("")
	assert.Error(t, err)

	_, err = parseRSAPublicKey("also-not-a-key!!!")
	assert.Error(t, err)
}

// TestNewAlipayClientRequiresAllCredentials 确保缺任一凭据都不放行支付。
func TestNewAlipayClientRequiresAllCredentials(t *testing.T) {
	origAppId := setting.AlipayAppId
	origPriv := setting.AlipayAppPrivateKey
	origPub := setting.AlipayPublicKey
	t.Cleanup(func() {
		setting.AlipayAppId = origAppId
		setting.AlipayAppPrivateKey = origPriv
		setting.AlipayPublicKey = origPub
	})

	privPEM, pubPEM := testAlipayKeyPair(t)

	setting.AlipayAppId = ""
	setting.AlipayAppPrivateKey = privPEM
	setting.AlipayPublicKey = pubPEM
	_, err := newAlipayClient()
	assert.Error(t, err, "缺少 APPID 应报错")

	setting.AlipayAppId = "2021000000000000"
	setting.AlipayAppPrivateKey = ""
	setting.AlipayPublicKey = pubPEM
	_, err = newAlipayClient()
	assert.Error(t, err, "缺少应用私钥应报错")

	setting.AlipayAppId = "2021000000000000"
	setting.AlipayAppPrivateKey = privPEM
	setting.AlipayPublicKey = ""
	_, err = newAlipayClient()
	assert.Error(t, err, "缺少支付宝公钥应报错")
}

// TestAlipayGatewaySwitchesOnSandbox 验证沙箱开关切换网关地址。
func TestAlipayGatewaySwitchesOnSandbox(t *testing.T) {
	orig := setting.AlipaySandbox
	t.Cleanup(func() { setting.AlipaySandbox = orig })

	setting.AlipaySandbox = false
	assert.Equal(t, "https://openapi.alipay.com/gateway.do", setting.AlipayGateway())

	setting.AlipaySandbox = true
	got := setting.AlipayGateway()
	assert.True(t, strings.Contains(got, "sandbox"), "沙箱网关应指向 sandbox 域名，实际: %s", got)
}

// TestAlipayPayURLPreparation 验证下单 URL 的构造结果符合支付宝要求。
// 这是「配好凭据就能直接上线」的关键验证：不依赖网络，只验证本地构造逻辑。
func TestAlipayPayURLPreparation(t *testing.T) {
	client := withAlipayTestClient(t)

	// 单价与最小充值使用默认值，与后台未配置时的落库默认一致
	origUnit := setting.AlipayUnitPrice
	origMin := setting.AlipayMinTopUp
	origReturn := setting.AlipayReturnUrl
	t.Cleanup(func() {
		setting.AlipayUnitPrice = origUnit
		setting.AlipayMinTopUp = origMin
		setting.AlipayReturnUrl = origReturn
	})
	setting.AlipayUnitPrice = 7.3
	setting.AlipayMinTopUp = 1
	oldAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://example.com"
	t.Cleanup(func() { system_setting.ServerAddress = oldAddress })

	payURL, err := genAlipayPayURL(client, "alipay-1-2-abcdef", decimal.NewFromInt(73), false)
	require.NoError(t, err)

	parsed, err := url.Parse(payURL)
	require.NoError(t, err)

	assert.Equal(t, "openapi.alipay.com", parsed.Host)
	assert.Equal(t, "/gateway.do", parsed.Path)

	q := parsed.Query()
	assert.Equal(t, "alipay.trade.page.pay", q.Get("method"))
	assert.Equal(t, "RSA2", q.Get("sign_type"))
	assert.Equal(t, "utf-8", q.Get("charset"))
	assert.Equal(t, "1.0", q.Get("version"))
	assert.Equal(t, client.appId, q.Get("app_id"))
	assert.NotEmpty(t, q.Get("sign"), "必须带签名")
	assert.NotEmpty(t, q.Get("notify_url"), "必须带异步通知地址")
	assert.NotEmpty(t, q.Get("return_url"), "必须带同步跳转地址")

	// 时间戳格式必须是 yyyy-MM-dd HH:mm:ss
	_, err = time.Parse("2006-01-02 15:04:05", q.Get("timestamp"))
	assert.NoError(t, err, "时间戳格式错误: %s", q.Get("timestamp"))

	// 业务参数必须含产品码与金额
	biz := q.Get("biz_content")
	assert.Contains(t, biz, `"product_code":"FAST_INSTANT_TRADE_PAY"`)
	assert.Contains(t, biz, `"total_amount":"73.00"`)
	assert.Contains(t, biz, `"out_trade_no":"alipay-1-2-abcdef"`)

	// 订单号不能含支付宝禁止的特殊字符
	assert.NotContains(t, biz, "/")
}

// newAlipayClientFromKeys 用显式密钥构造客户端，供测试构造「外来密钥」场景。
func newAlipayClientFromKeys(appId string, appPrivateKeyPEM string, alipayPublicKey *rsa.PublicKey) (*alipayClient, error) {
	privKey, err := parseRSAPrivateKey(appPrivateKeyPEM)
	if err != nil {
		return nil, err
	}
	return &alipayClient{
		appId:         appId,
		appPrivateKey: privKey,
		alipayPubKey:  alipayPublicKey,
	}, nil
}

func TestAlipayCheckoutAndNotification(t *testing.T) {
	client := withAlipayTestClient(t)
	confirmPaymentComplianceForTest(t)
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMain, oldLog := common.MainDatabaseType(), common.LogDatabaseType()
	oldRedis, oldBatch := common.RedisEnabled, common.BatchUpdateEnabled
	oldUnit, oldMin, oldSeller := setting.AlipayUnitPrice, setting.AlipayMinTopUp, setting.AlipaySellerId
	oldAddress, oldCallback, oldNotify, oldReturn := system_setting.ServerAddress, operation_setting.CustomCallbackAddress, setting.AlipayNotifyUrl, setting.AlipayReturnUrl
	oldQuota, oldDisplay := common.QuotaPerUnit, operation_setting.GetGeneralSetting().QuotaDisplayType
	oldDiscount, oldRatio := operation_setting.GetPaymentSetting().AmountDiscount, common.TopupGroupRatio2JSONString()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMain, oldLog)
		common.RedisEnabled, common.BatchUpdateEnabled = oldRedis, oldBatch
		setting.AlipayUnitPrice, setting.AlipayMinTopUp, setting.AlipaySellerId = oldUnit, oldMin, oldSeller
		system_setting.ServerAddress, operation_setting.CustomCallbackAddress, setting.AlipayNotifyUrl, setting.AlipayReturnUrl = oldAddress, oldCallback, oldNotify, oldReturn
		common.QuotaPerUnit, operation_setting.GetGeneralSetting().QuotaDisplayType = oldQuota, oldDisplay
		operation_setting.GetPaymentSetting().AmountDiscount = oldDiscount
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(oldRatio))
	})
	common.RedisEnabled, common.BatchUpdateEnabled = false, false
	common.QuotaPerUnit = 500000
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	setting.AlipayUnitPrice, setting.AlipayMinTopUp, setting.AlipaySellerId = 7.3, 1, "2088000000000000"
	system_setting.ServerAddress, operation_setting.CustomCallbackAddress = "https://gateway.example.com", ""
	setting.AlipayNotifyUrl, setting.AlipayReturnUrl = "", ""
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{10: 0.9}
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"alipay_test":1.5}`))

	for _, dialect := range []struct {
		kind, env string
		dbType    common.DatabaseType
	}{
		{"sqlite", "", common.DatabaseTypeSQLite}, {"mysql", "TEST_MYSQL_DSN", common.DatabaseTypeMySQL}, {"postgres", "TEST_POSTGRES_DSN", common.DatabaseTypePostgreSQL},
	} {
		t.Run(dialect.kind, func(t *testing.T) {
			dsn := os.Getenv(dialect.env)
			if dialect.env != "" && dsn == "" {
				t.Skip("configure " + dialect.env + " for database verification")
			}
			db, _ := newAuditTestDatabase(t, dialect.kind, dsn)
			logDB, _ := newAuditTestDatabase(t, dialect.kind, dsn)
			model.DB, model.LOG_DB = db, logDB
			common.SetDatabaseTypes(dialect.dbType, dialect.dbType)
			require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}))
			require.NoError(t, logDB.AutoMigrate(&model.Log{}))
			var version string
			query := "SELECT version()"
			if dialect.kind == "sqlite" {
				query = "SELECT sqlite_version()"
			}
			require.NoError(t, db.Raw(query).Scan(&version).Error)
			t.Logf("database version: %s", version)
			require.NoError(t, db.Create(&model.User{Id: 101, Username: "alipay_test", Group: "alipay_test", Quota: 100, Status: common.UserStatusEnabled}).Error)
			invoke := func(handler gin.HandlerFunc, path, body, contentType string) *httptest.ResponseRecorder {
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Set("id", 101)
				ctx.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
				ctx.Request.Header.Set("Content-Type", contentType)
				handler(ctx)
				return recorder
			}
			info := invoke(GetTopUpInfo, "/api/user/topup/info", "", "application/json")
			var topupInfo struct {
				Data struct {
					EnableAlipay bool                `json:"enable_alipay_topup"`
					Methods      []map[string]string `json:"pay_methods"`
				}
			}
			require.NoError(t, common.Unmarshal(info.Body.Bytes(), &topupInfo))
			assert.True(t, topupInfo.Data.EnableAlipay)
			var official int
			for _, method := range topupInfo.Data.Methods {
				if method["type"] == model.PaymentMethodAlipay {
					official++
				}
			}
			assert.Equal(t, 1, official)

			quote := invoke(RequestAlipayAmount, "/api/user/alipay/amount", `{"amount":10}`, "application/json")
			assert.JSONEq(t, `{"message":"success","data":"98.55"}`, quote.Body.String())
			created := invoke(RequestAlipayPay, "/api/user/alipay/pay", `{"amount":10,"payment_method":"alipay_direct"}`, "application/json")
			var response struct {
				Message string
				Data    struct {
					PayLink   string `json:"pay_link"`
					QRPayLink string `json:"qr_pay_link"`
					ExpiresAt int64  `json:"expires_at"`
					TradeNo   string `json:"trade_no"`
				}
			}
			require.NoError(t, common.Unmarshal(created.Body.Bytes(), &response))
			require.Equal(t, "success", response.Message, created.Body.String())
			payURL, err := url.Parse(response.Data.PayLink)
			require.NoError(t, err)
			assert.Equal(t, "https://gateway.example.com/api/alipay/notify", payURL.Query().Get("notify_url"))
			var biz map[string]string
			require.NoError(t, common.UnmarshalJsonStr(payURL.Query().Get("biz_content"), &biz))
			assert.Equal(t, "98.55", biz["total_amount"])
			assert.Equal(t, setting.AlipaySellerId, biz["seller_id"])
			embeddedURL, err := url.Parse(response.Data.QRPayLink)
			require.NoError(t, err)
			require.Equal(t, "alipay.trade.page.pay", embeddedURL.Query().Get("method"))
			require.Empty(t, embeddedURL.Query().Get("return_url"), "embedded checkout must not navigate into the wallet")
			var embeddedBiz map[string]string
			require.NoError(t, common.UnmarshalJsonStr(embeddedURL.Query().Get("biz_content"), &embeddedBiz))
			assert.Equal(t, "4", embeddedBiz["qr_pay_mode"])
			assert.Equal(t, "256", embeddedBiz["qrcode_width"])
			assert.Equal(t, response.Data.TradeNo, embeddedBiz["out_trade_no"])
			assert.Equal(t, biz["total_amount"], embeddedBiz["total_amount"])
			params := map[string]string{}
			for k := range embeddedURL.Query() {
				params[k] = embeddedURL.Query().Get(k)
			}
			assert.NoError(t, client.verify(alipaySignContent(params), embeddedURL.Query().Get("sign")))
			assert.Greater(t, response.Data.ExpiresAt, common.GetTimestamp())
			topup := model.GetTopUpByTradeNo(response.Data.TradeNo)
			require.NotNil(t, topup)
			assert.EqualValues(t, 7500000, topup.Amount)
			assert.Equal(t, 98.55, topup.Money)

			statusRouter := gin.New()
			statusRouter.Use(func(c *gin.Context) { c.Set("id", 101) })
			statusRouter.GET("/alipay/orders/:trade_no", RequestAlipayOrderStatus)
			readStatus := func(tradeNo string) *httptest.ResponseRecorder {
				recorder := httptest.NewRecorder()
				statusRouter.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/alipay/orders/"+tradeNo, nil))
				return recorder
			}
			pendingStatus := readStatus(response.Data.TradeNo)
			assert.JSONEq(t, `{"success":true,"message":"","data":{"status":"pending"}}`, pendingStatus.Body.String())
			require.NoError(t, db.Model(&model.TopUp{}).Where("trade_no = ?", response.Data.TradeNo).Update("user_id", 102).Error)
			assert.Equal(t, http.StatusNotFound, readStatus(response.Data.TradeNo).Code)
			require.NoError(t, db.Model(&model.TopUp{}).Where("trade_no = ?", response.Data.TradeNo).Update("user_id", 101).Error)
			require.NoError(t, db.Model(&model.TopUp{}).Where("trade_no = ?", response.Data.TradeNo).Update("payment_provider", model.PaymentProviderEpay).Error)
			assert.Equal(t, http.StatusNotFound, readStatus(response.Data.TradeNo).Code)
			require.NoError(t, db.Model(&model.TopUp{}).Where("trade_no = ?", response.Data.TradeNo).Update("payment_provider", model.PaymentProviderAlipay).Error)
			assert.Equal(t, http.StatusNotFound, readStatus("missing-order").Code)

			// Notification signatures exclude sign_type, independently from request signing.
			notify := func(changes map[string]string, tamper bool, duplicate bool) *httptest.ResponseRecorder {
				values := url.Values{"app_id": {client.appId}, "seller_id": {setting.AlipaySellerId}, "trade_no": {"202600000001"}, "out_trade_no": {response.Data.TradeNo}, "total_amount": {"98.550"}, "trade_status": {"TRADE_SUCCESS"}, "sign_type": {"RSA2"}, "charset": {"utf-8"}}
				for k, v := range changes {
					values.Set(k, v)
				}
				params := map[string]string{}
				for k := range values {
					if k != "sign_type" {
						params[k] = values.Get(k)
					}
				}
				sign, err := client.sign(params)
				require.NoError(t, err)
				values.Set("sign", sign)
				if tamper {
					values.Set("total_amount", "100.00")
				}
				if duplicate {
					values.Add("total_amount", "100.00")
				}
				return invoke(AlipayNotify, "/api/alipay/notify", values.Encode(), "application/x-www-form-urlencoded")
			}
			for _, tc := range []struct {
				name              string
				changes           map[string]string
				tamper, duplicate bool
			}{
				{name: "tampered amount", tamper: true}, {name: "duplicate parameters", duplicate: true},
				{name: "wrong app", changes: map[string]string{"app_id": "another-app"}},
				{name: "wrong seller", changes: map[string]string{"seller_id": "2088111111111111"}},
				{name: "wrong amount", changes: map[string]string{"total_amount": "10.00"}},
				{name: "missing trade id", changes: map[string]string{"trade_no": ""}},
				{name: "wrong algorithm", changes: map[string]string{"sign_type": "RSA"}},
				{name: "wrong charset", changes: map[string]string{"charset": "gbk"}},
				{name: "unknown order", changes: map[string]string{"out_trade_no": "missing-order"}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					assert.Equal(t, "failure", notify(tc.changes, tc.tamper, tc.duplicate).Body.String())
				})
			}
			assert.Equal(t, common.TopUpStatusPending, model.GetTopUpByTradeNo(response.Data.TradeNo).Status)
			var user model.User
			require.NoError(t, db.First(&user, 101).Error)
			assert.Equal(t, 100, user.Quota)
			assert.Equal(t, "success", notify(map[string]string{"trade_status": "WAIT_BUYER_PAY"}, false, false).Body.String())
			assert.Equal(t, common.TopUpStatusPending, model.GetTopUpByTradeNo(response.Data.TradeNo).Status)
			// Changing pricing after checkout must not change the credited quota.
			common.QuotaPerUnit = 1000000
			assert.Equal(t, "success", notify(nil, false, false).Body.String())
			assert.Equal(t, "success", notify(map[string]string{"trade_status": "TRADE_FINISHED"}, false, false).Body.String())
			common.QuotaPerUnit = 500000
			require.NoError(t, db.First(&user, 101).Error)
			assert.Equal(t, 7500100, user.Quota)
			assert.Equal(t, common.TopUpStatusSuccess, model.GetTopUpByTradeNo(response.Data.TradeNo).Status)
			assert.JSONEq(t, `{"success":true,"message":"","data":{"status":"success"}}`, readStatus(response.Data.TradeNo).Body.String())
			var logs int64
			require.NoError(t, logDB.Model(&model.Log{}).Where("user_id = ? AND type = ?", 101, model.LogTypeTopup).Count(&logs).Error)
			assert.EqualValues(t, 1, logs)

			// A wallet limit failure rolls back the order transition and can be retried.
			pending := model.TopUp{UserId: 101, Amount: 100, Money: 1, TradeNo: "capacity-order", PaymentMethod: model.PaymentMethodAlipay, PaymentProvider: model.PaymentProviderAlipay, Status: common.TopUpStatusPending}
			require.NoError(t, pending.Insert())
			require.NoError(t, db.Model(&model.User{}).Where("id = ?", 101).Update("quota", common.MaxWalletQuota-10).Error)
			require.Error(t, model.RechargeAlipay(pending.TradeNo, decimal.NewFromInt(1), ""))
			assert.Equal(t, common.TopUpStatusPending, model.GetTopUpByTradeNo(pending.TradeNo).Status)
			require.NoError(t, db.Model(&model.User{}).Where("id = ?", 101).Update("quota", 0).Error)
			require.NoError(t, model.RechargeAlipay(pending.TradeNo, decimal.NewFromInt(1), ""))
			require.NoError(t, db.First(&user, 101).Error)
			assert.Equal(t, 100, user.Quota)

			manual := model.TopUp{UserId: 101, Amount: 200, Money: 7.30, TradeNo: "manual-alipay", PaymentMethod: model.PaymentMethodAlipay, PaymentProvider: model.PaymentProviderAlipay, Status: common.TopUpStatusPending}
			require.NoError(t, manual.Insert())
			require.NoError(t, model.ManualCompleteTopUp(manual.TradeNo, ""))
			require.NoError(t, model.RechargeAlipay(manual.TradeNo, decimal.NewFromFloat(7.3), ""))
			require.NoError(t, db.First(&user, 101).Error)
			assert.Equal(t, 300, user.Quota)

			concurrent := model.TopUp{UserId: 101, Amount: 300, Money: 1, TradeNo: "concurrent-alipay", PaymentMethod: model.PaymentMethodAlipay, PaymentProvider: model.PaymentProviderAlipay, Status: common.TopUpStatusPending}
			require.NoError(t, concurrent.Insert())
			results := make(chan error, 2)
			var workers sync.WaitGroup
			for range 2 {
				workers.Go(func() { results <- model.RechargeAlipay(concurrent.TradeNo, decimal.NewFromInt(1), "") })
			}
			workers.Wait()
			close(results)
			for err := range results {
				if dialect.kind != "sqlite" {
					assert.NoError(t, err)
				}
			}
			require.NoError(t, model.RechargeAlipay(concurrent.TradeNo, decimal.NewFromInt(1), ""))
			require.NoError(t, db.First(&user, 101).Error)
			assert.Equal(t, 600, user.Quota, "concurrent deliveries must credit once")

			// Official and aggregator orders must not be settled through each other's callback.
			wrong := model.TopUp{UserId: 101, Amount: 100, Money: 1, TradeNo: "epay-order", PaymentMethod: "alipay", PaymentProvider: model.PaymentProviderEpay, Status: common.TopUpStatusPending}
			require.NoError(t, wrong.Insert())
			require.ErrorIs(t, model.RechargeAlipay(wrong.TradeNo, decimal.NewFromInt(1), ""), model.ErrPaymentMethodMismatch)

			operation_setting.GetPaymentSetting().ComplianceConfirmed = false
			assert.Equal(t, http.StatusForbidden, invoke(RequestAlipayPay, "/api/user/alipay/pay", `{"amount":10,"payment_method":"alipay_direct"}`, "application/json").Code)
			assert.Equal(t, http.StatusForbidden, invoke(RequestAlipayAmount, "/api/user/alipay/amount", `{"amount":10}`, "application/json").Code)
			operation_setting.GetPaymentSetting().ComplianceConfirmed = true
		})
	}
}

func TestAlipayQuoteBoundaries(t *testing.T) {
	oldPrice, oldMin, oldQuota := setting.AlipayUnitPrice, setting.AlipayMinTopUp, common.QuotaPerUnit
	oldDisplay, oldDiscount := operation_setting.GetGeneralSetting().QuotaDisplayType, operation_setting.GetPaymentSetting().AmountDiscount
	oldRatio := common.TopupGroupRatio2JSONString()
	t.Cleanup(func() {
		setting.AlipayUnitPrice, setting.AlipayMinTopUp, common.QuotaPerUnit = oldPrice, oldMin, oldQuota
		operation_setting.GetGeneralSetting().QuotaDisplayType = oldDisplay
		operation_setting.GetPaymentSetting().AmountDiscount = oldDiscount
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(oldRatio))
	})
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"alipay_boundary":1}`))
	for _, tc := range []struct {
		name, display string
		amount        int64
		price, unit   float64
		quota         int
		money         string
		invalid       bool
	}{
		{name: "currency", display: operation_setting.QuotaDisplayTypeUSD, amount: 10, price: 7.3, unit: 500000, quota: 5000000, money: "73.00"},
		{name: "token display", display: operation_setting.QuotaDisplayTypeTokens, amount: 5000000, price: 7.3, unit: 500000, quota: 5000000, money: "73.00"},
		{name: "fractional units preserve exact tokens", display: operation_setting.QuotaDisplayTypeTokens, amount: 4, price: 7.3, unit: 3, quota: 4, money: "9.73"},
		{name: "one cent", amount: 1, price: 0.01, unit: 500000, quota: 500000, money: "0.01"},
		{name: "zero", amount: 0, price: 7.3, unit: 500000, invalid: true},
		{name: "negative", amount: -1, price: 7.3, unit: 500000, invalid: true},
		{name: "too many units", amount: 10001, price: 7.3, unit: 500000, invalid: true},
		{name: "wallet overflow", amount: 10000, price: 7.3, unit: 1e15, invalid: true},
		{name: "non finite price", amount: 10, price: math.Inf(1), unit: 500000, invalid: true},
		{name: "nan price", amount: 10, price: math.NaN(), unit: 500000, invalid: true},
		{name: "zero price", amount: 10, price: 0, unit: 500000, invalid: true},
		{name: "below one cent", amount: 1, price: 0.001, unit: 500000, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setting.AlipayUnitPrice, setting.AlipayMinTopUp, common.QuotaPerUnit = tc.price, 1, tc.unit
			operation_setting.GetGeneralSetting().QuotaDisplayType = tc.display
			operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{}
			money, quota, err := alipayTopUpQuote(tc.amount, "alipay_boundary")
			if tc.invalid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.quota, quota)
			assert.Equal(t, tc.money, money.StringFixed(2))
		})
	}
}

func TestAlipayConfigurationValidation(t *testing.T) {
	for _, tc := range []struct {
		key, value string
		invalid    bool
	}{
		{"AlipayUnitPrice", "NaN", true}, {"AlipayUnitPrice", "+Inf", true}, {"AlipayUnitPrice", "-1", true}, {"AlipayUnitPrice", "7.3", false},
		{"AlipayMinTopUp", "1.5", true}, {"AlipayMinTopUp", "0", true}, {"AlipayMinTopUp", "10001", true}, {"AlipayMinTopUp", "1", false},
		{"AlipayAppPrivateKey", "garbage", true}, {"AlipayPublicKey", "garbage", true},
		{"AlipaySellerId", "not-a-seller", true}, {"AlipaySellerId", "2088000000000000", false},
		{"AlipayNotifyUrl", "/api/alipay/notify", true}, {"AlipayReturnUrl", "javascript:alert(1)", true}, {"AlipayNotifyUrl", "https://example.com/api/alipay/notify", false}, {"AlipayNotifyUrl", "", false},
	} {
		t.Run(tc.key+"/"+tc.value, func(t *testing.T) {
			err := validateAlipayOption(tc.key, tc.value)
			if tc.invalid {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
