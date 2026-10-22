package setting

// 支付宝官方直连（电脑网站支付）配置。
// 凭据从支付宝开放平台「开发设置」中获取，加签方式为密钥模式、算法 RSA2。
var (
	// AlipayAppId 是开放平台应用的 APPID。
	AlipayAppId = ""
	// AlipayAppPrivateKey 是应用私钥（PKCS#8 PEM），用于对请求加签。
	AlipayAppPrivateKey = ""
	// AlipayPublicKey 是支付宝公钥（单行 base64 或 PEM），用于验签异步通知。
	AlipayPublicKey = ""
	// AlipaySellerId 为签约收款账号的支付宝用户 ID（PID）。
	AlipaySellerId = ""
	// AlipayUnitPrice 是每单位额度的人民币售价。
	AlipayUnitPrice = 7.3
	// AlipayMinTopUp 是最小充值额度。
	AlipayMinTopUp = 1
	// AlipayNotifyUrl 是支付结果异步通知地址，留空则使用配置的站点地址。
	AlipayNotifyUrl = ""
	// AlipayReturnUrl 是支付完成后同步跳转地址，留空则使用配置的站点地址。
	AlipayReturnUrl = ""
	// AlipaySandbox 为 true 时使用沙箱网关，仅用于联调。
	AlipaySandbox = false
)

// AlipayGateway 返回当前使用的支付宝网关地址。
func AlipayGateway() string {
	if AlipaySandbox {
		return "https://openapi-sandbox.dl.alipaydev.com/gateway.do"
	}
	return "https://openapi.alipay.com/gateway.do"
}
