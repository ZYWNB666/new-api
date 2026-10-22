package controller

import (
	"errors"
	"fmt"
	"math"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/thanhpk/randstr"
	"gorm.io/gorm"
)

type AlipayPayRequest struct {
	Amount        int64  `json:"amount"`
	PaymentMethod string `json:"payment_method"`
}

// alipayTopUpQuote returns the actual CNY charge and the quota to freeze on the
// order. Discounts reduce the payment, never the quota purchased.
func alipayTopUpQuote(amount int64, group string) (decimal.Decimal, int, error) {
	for _, value := range []float64{common.QuotaPerUnit, setting.AlipayUnitPrice} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
			return decimal.Zero, 0, fmt.Errorf("支付宝计价配置无效")
		}
	}
	units := decimal.NewFromInt(amount)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		units = units.Div(decimal.NewFromFloat(common.QuotaPerUnit))
	}
	if setting.AlipayMinTopUp < 1 || units.LessThan(decimal.NewFromInt(int64(setting.AlipayMinTopUp))) || units.GreaterThan(decimal.NewFromInt(10000)) {
		return decimal.Zero, 0, fmt.Errorf("充值数量须在最小充值额度与 10000 单位之间")
	}
	ratio := common.GetTopupGroupRatio(group)
	if ratio == 0 {
		ratio = 1
	}
	discount := 1.0
	if value, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(amount)]; ok {
		discount = value
	}
	for _, value := range []float64{ratio, discount} {
		if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
			return decimal.Zero, 0, fmt.Errorf("充值倍率或折扣配置无效")
		}
	}
	creditedUnits := units.Mul(decimal.NewFromFloat(ratio))
	creditedTokens := decimal.NewFromInt(amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit))
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		creditedTokens = decimal.NewFromInt(amount)
	}
	quota, err := common.WalletQuotaFromDecimalStrict(creditedTokens.Mul(decimal.NewFromFloat(ratio)))
	if err != nil || quota <= 0 {
		return decimal.Zero, 0, model.ErrInvalidTopUpQuota
	}
	money := creditedUnits.Mul(decimal.NewFromFloat(setting.AlipayUnitPrice)).Mul(decimal.NewFromFloat(discount)).Round(2)
	if money.LessThan(decimal.NewFromFloat(0.01)) || money.GreaterThan(decimal.NewFromInt(100000000)) {
		return decimal.Zero, 0, fmt.Errorf("支付宝支付金额超出范围")
	}
	return money, quota, nil
}

func getAlipayMinTopup() int64 {
	minimum := decimal.NewFromInt(int64(setting.AlipayMinTopUp))
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		if math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) || common.QuotaPerUnit <= 0 {
			return 0
		}
		minimum = minimum.Mul(decimal.NewFromFloat(common.QuotaPerUnit)).Ceil()
	}
	quota, err := common.WalletQuotaFromDecimalStrict(minimum)
	if err != nil {
		return 0
	}
	return int64(quota)
}

func RequestAlipayAmount(c *gin.Context) {
	requestAlipayCheckout(c, false)
}

func RequestAlipayPay(c *gin.Context) {
	requestAlipayCheckout(c, true)
}

func requestAlipayCheckout(c *gin.Context, createOrder bool) {
	if !isAlipayTopUpEnabled() {
		c.JSON(http.StatusForbidden, gin.H{"message": "error", "data": "支付宝支付未启用"})
		return
	}
	var req AlipayPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if createOrder && req.PaymentMethod != model.PaymentMethodAlipay {
		c.JSON(http.StatusBadRequest, gin.H{"message": "error", "data": "不支持的支付渠道"})
		return
	}
	id := c.GetInt("id")
	user, err := model.GetUserById(id, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	money, quota, err := alipayTopUpQuote(req.Amount, user.Group)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	if rejectInvalidCreditedQuota(c, id, decimal.NewFromInt(int64(quota))) {
		return
	}
	if !createOrder {
		c.JSON(http.StatusOK, gin.H{"message": "success", "data": money.StringFixed(2)})
		return
	}
	client, err := newAlipayClient()
	if err != nil {
		logger.LogError(c.Request.Context(), "支付宝客户端配置无效: "+err.Error())
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付宝支付未正确配置"})
		return
	}
	tradeNo := "alipay-" + randstr.String(32)
	payURL, err := genAlipayPayURL(client, tradeNo, money, false)
	if err != nil {
		logger.LogError(c.Request.Context(), "支付宝下单失败: "+err.Error())
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	qrPayURL, err := genAlipayPayURL(client, tradeNo, money, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	paidMoney, _ := money.Float64()
	topUp := &model.TopUp{
		UserId: id, Amount: int64(quota), Money: paidMoney, TradeNo: tradeNo,
		PaymentMethod: model.PaymentMethodAlipay, PaymentProvider: model.PaymentProviderAlipay,
		CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("支付宝创建订单失败 trade_no=%s: %v", tradeNo, err))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": gin.H{"pay_link": payURL, "qr_pay_link": qrPayURL, "trade_no": tradeNo, "expires_at": topUp.CreateTime + 30*60}})
}

// Page pay can be embedded or opened as a full checkout. Only verified
// asynchronous notifications may credit funds; browser navigation is not proof of payment.
func genAlipayPayURL(client *alipayClient, tradeNo string, money decimal.Decimal, embedded bool) (string, error) {
	notifyURL := strings.TrimSpace(setting.AlipayNotifyUrl)
	if notifyURL == "" {
		notifyURL = strings.TrimRight(service.GetCallbackAddress(), "/") + "/api/alipay/notify"
	}
	returnURL := strings.TrimSpace(setting.AlipayReturnUrl)
	if returnURL == "" {
		returnURL = paymentReturnPath("/wallet")
	}
	for _, raw := range []string{notifyURL, returnURL} {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil || parsed.Fragment != "" {
			return "", fmt.Errorf("支付宝回调地址必须为完整的 HTTP/HTTPS URL")
		}
	}
	bizContent := map[string]string{
		"out_trade_no": tradeNo, "total_amount": money.StringFixed(2), "subject": "账户充值",
		"product_code": "FAST_INSTANT_TRADE_PAY", "timeout_express": "30m", "seller_id": strings.TrimSpace(setting.AlipaySellerId),
	}
	if embedded {
		bizContent["qr_pay_mode"] = "4"
		bizContent["qrcode_width"] = "256"
	}
	biz, err := common.Marshal(bizContent)
	if err != nil {
		return "", err
	}
	params := map[string]string{
		"app_id": client.appId, "method": "alipay.trade.page.pay", "format": "JSON", "charset": "utf-8", "sign_type": "RSA2",
		"timestamp": time.Now().In(time.FixedZone("CST", 8*60*60)).Format("2006-01-02 15:04:05"), "version": "1.0",
		"return_url": returnURL, "notify_url": notifyURL, "biz_content": string(biz),
	}
	if embedded {
		// Keep the frame on Alipay's completion screen; never load the whole wallet inside it.
		delete(params, "return_url")
	}
	sign, err := client.sign(params)
	if err != nil {
		return "", err
	}
	values := url.Values{"sign": {sign}}
	for k, v := range params {
		values.Set(k, v)
	}
	return setting.AlipayGateway() + "?" + values.Encode(), nil
}

// AlipayNotify accepts signed form POSTs only. Acknowledgement follows commit;
// transient storage failures return failure so Alipay retries the notification.
func AlipayNotify(c *gin.Context) {
	if !isAlipayNotifyEnabled() {
		c.String(http.StatusForbidden, "failure")
		return
	}
	contentType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || contentType != "application/x-www-form-urlencoded" {
		c.String(http.StatusBadRequest, "failure")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	if err := c.Request.ParseForm(); err != nil {
		c.String(http.StatusBadRequest, "failure")
		return
	}
	params := make(map[string]string, len(c.Request.PostForm))
	for key, values := range c.Request.PostForm {
		if len(values) != 1 {
			c.String(http.StatusBadRequest, "failure")
			return
		}
		params[key] = values[0]
	}
	if params["sign_type"] != "RSA2" || !strings.EqualFold(params["charset"], "utf-8") || params["sign"] == "" {
		c.String(http.StatusBadRequest, "failure")
		return
	}
	publicKey, err := parseRSAPublicKey(setting.AlipayPublicKey)
	if err != nil || publicKey.N.BitLen() < 2048 {
		c.String(http.StatusServiceUnavailable, "failure")
		return
	}
	client := &alipayClient{alipayPubKey: publicKey}
	sign := params["sign"]
	delete(params, "sign_type")
	if err := client.verify(alipaySignContent(params), sign); err != nil {
		logger.LogWarn(c.Request.Context(), "支付宝回调验签失败")
		c.String(http.StatusBadRequest, "failure")
		return
	}
	if params["app_id"] != strings.TrimSpace(setting.AlipayAppId) || params["seller_id"] != strings.TrimSpace(setting.AlipaySellerId) || params["out_trade_no"] == "" || params["trade_no"] == "" {
		c.String(http.StatusBadRequest, "failure")
		return
	}
	if params["trade_status"] != "TRADE_SUCCESS" && params["trade_status"] != "TRADE_FINISHED" {
		c.String(http.StatusOK, "success")
		return
	}
	money, err := decimal.NewFromString(params["total_amount"])
	if err != nil || !money.IsPositive() || !money.Equal(money.Round(2)) {
		c.String(http.StatusBadRequest, "failure")
		return
	}
	if err := model.RechargeAlipay(params["out_trade_no"], money, c.ClientIP()); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("支付宝回调入账失败 trade_no=%q: %v", params["out_trade_no"], err))
		c.String(http.StatusInternalServerError, "failure")
		return
	}
	c.String(http.StatusOK, "success")
}

// RequestAlipayOrderStatus reports settlement recorded for the authenticated
// owner's order. Reading status never changes quota or trusts browser messages.
func RequestAlipayOrderStatus(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if c.GetInt("id") <= 0 {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	tradeNo := c.Param("trade_no")
	if tradeNo == "" || len(tradeNo) > 64 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "订单号无效"})
		return
	}
	status, err := model.GetUserAlipayOrderStatus(c.GetInt("id"), tradeNo)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "订单不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "查询订单失败"})
		return
	}
	common.ApiSuccess(c, gin.H{"status": status})
}
