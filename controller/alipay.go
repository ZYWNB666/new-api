package controller

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/setting"
)

// alipayClient 封装电脑网站支付所需的加签与验签能力。
// 使用标准库实现密钥模式 RSA2（RSA + SHA256）。
type alipayClient struct {
	appId         string
	appPrivateKey *rsa.PrivateKey
	alipayPubKey  *rsa.PublicKey
}

// newAlipayClient 从配置构造客户端。密钥格式错误时返回错误，
// 由调用方转成对用户友好的提示，避免把密钥细节写进响应。
func newAlipayClient() (*alipayClient, error) {
	appId := strings.TrimSpace(setting.AlipayAppId)
	if appId == "" {
		return nil, fmt.Errorf("支付宝 APPID 未配置")
	}

	privKey, err := parseRSAPrivateKey(setting.AlipayAppPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("支付宝应用私钥解析失败: %w", err)
	}

	pubKey, err := parseRSAPublicKey(setting.AlipayPublicKey)
	if err != nil {
		return nil, fmt.Errorf("支付宝公钥解析失败: %w", err)
	}
	if privKey.N.BitLen() < 2048 || pubKey.N.BitLen() < 2048 {
		return nil, fmt.Errorf("RSA2 密钥必须至少为 2048 位")
	}

	return &alipayClient{
		appId:         appId,
		appPrivateKey: privKey,
		alipayPubKey:  pubKey,
	}, nil
}

// parseRSAPrivateKey 兼容两种常见格式：带 PEM 头尾的 PKCS#8/PKCS#1，
// 以及开放平台「密钥工具」导出的裸 base64 单行字符串。
func parseRSAPrivateKey(raw string) (*rsa.PrivateKey, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("私钥为空")
	}

	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		// 裸 base64：按 PKCS#8 尝试
		der, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(raw), ""))
		if err != nil {
			return nil, fmt.Errorf("不是合法的 PEM 或 base64 私钥")
		}
		key, err := x509.ParsePKCS8PrivateKey(der)
		if err != nil {
			return nil, err
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("私钥不是 RSA 类型")
		}
		return rsaKey, nil
	}

	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
		return nil, fmt.Errorf("私钥不是 RSA 类型")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	return nil, fmt.Errorf("无法解析私钥内容")
}

// parseRSAPublicKey 兼容 PEM 与裸 base64 单行两种格式。
func parseRSAPublicKey(raw string) (*rsa.PublicKey, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("公钥为空")
	}

	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		der, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(raw), ""))
		if err != nil {
			return nil, fmt.Errorf("不是合法的 PEM 或 base64 公钥")
		}
		pub, err := x509.ParsePKIXPublicKey(der)
		if err != nil {
			return nil, err
		}
		rsaKey, ok := pub.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("公钥不是 RSA 类型")
		}
		return rsaKey, nil
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		// 兼容 PKCS#1 形式的公钥
		if rsaKey, err2 := x509.ParsePKCS1PublicKey(block.Bytes); err2 == nil {
			return rsaKey, nil
		}
		return nil, err
	}
	rsaKey, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("公钥不是 RSA 类型")
	}
	return rsaKey, nil
}

// sign 按支付宝规范对待签名参数排序拼接后加签。
// 规则：剔除 sign 本身与空值参数，按参数名 ASCII 升序，以 & 连接 k=v。
func (c *alipayClient) sign(params map[string]string) (string, error) {
	signed := alipaySignContent(params)
	digest := sha256.Sum256([]byte(signed))
	sig, err := rsa.SignPKCS1v15(nil, c.appPrivateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// verify 校验支付宝返回或异步通知的签名。
// notifyContent 必须是已剔除 sign 字段的原始待验串。
func (c *alipayClient) verify(notifyContent string, sign string) error {
	digest := sha256.Sum256([]byte(notifyContent))
	sigBytes, err := base64.StdEncoding.DecodeString(sign)
	if err != nil {
		return fmt.Errorf("签名不是合法 base64: %w", err)
	}
	return rsa.VerifyPKCS1v15(c.alipayPubKey, crypto.SHA256, digest[:], sigBytes)
}

// alipaySignContent 生成待签名字符串：按 key 升序、跳过空值与 sign。请求签名必须包含 sign_type；通知验签由调用方移除它。
func alipaySignContent(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if v == "" || k == "sign" {
			continue
		}
		keys = append(keys, k)
	}
	slices.Sort(keys)

	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(params[k])
	}
	return b.String()
}

// validateAlipayOption rejects unusable configuration before it is persisted.
func validateAlipayOption(key, value string) error {
	if value == "" && key != "AlipayUnitPrice" && key != "AlipayMinTopUp" && key != "AlipaySandbox" {
		return nil
	}
	switch key {
	case "AlipayUnitPrice":
		number, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number <= 0 || number > 100000000 {
			return fmt.Errorf("支付宝单价必须为有效的正数")
		}
	case "AlipayMinTopUp":
		number, err := strconv.Atoi(value)
		if err != nil || number < 1 || number > 10000 {
			return fmt.Errorf("支付宝最小充值额度须为 1 至 10000 的整数")
		}
	case "AlipayAppId", "AlipaySellerId":
		if len(value) < 16 || len(value) > 32 || (key == "AlipaySellerId" && (len(value) != 16 || !strings.HasPrefix(value, "2088"))) {
			return fmt.Errorf("支付宝 APPID 或收款 PID 格式无效")
		}
		for _, digit := range value {
			if digit < '0' || digit > '9' {
				return fmt.Errorf("支付宝 APPID 或收款 PID 格式无效")
			}
		}
	case "AlipayAppPrivateKey":
		privateKey, err := parseRSAPrivateKey(value)
		if err != nil || privateKey.N.BitLen() < 2048 {
			return fmt.Errorf("支付宝应用私钥必须为至少 2048 位的 RSA 密钥")
		}
		if err := privateKey.Validate(); err != nil {
			return fmt.Errorf("支付宝应用私钥无效")
		}
	case "AlipayPublicKey":
		publicKey, err := parseRSAPublicKey(value)
		if err != nil || publicKey.N.BitLen() < 2048 {
			return fmt.Errorf("支付宝公钥必须为至少 2048 位的 RSA 密钥")
		}
	case "AlipayNotifyUrl", "AlipayReturnUrl":
		parsed, err := url.Parse(value)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.User != nil || parsed.Fragment != "" {
			return fmt.Errorf("支付宝回调地址必须为完整的 HTTP/HTTPS URL")
		}
	case "AlipaySandbox":
		if value != "true" && value != "false" {
			return fmt.Errorf("支付宝沙箱设置必须为布尔值")
		}
	}
	return nil
}
