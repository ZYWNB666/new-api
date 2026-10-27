# 05 · OAuth 代理支持

- **状态**：活跃
- **引入提交**：`cf0ef5fd7`
- **影响面**：`oauth/` 全部 provider

## 功能说明

允许为特定 OAuth provider 的 HTTP 请求单独配置代理，避免所有第三方登录流量都走全局代理或都直连。

## 实现要点

- 环境变量：
  - `OAUTH_PROXY_URL`：代理地址。
  - `OAUTH_PROXY_PROVIDERS`：逗号分隔的 provider 列表，只有命中的才走代理。
- `oauth/http_client.go`：
  - `newOAuthHTTPClient` / `configuredOAuthHTTPClient`：命中列表用 `http.ProxyURL`，否则默认 client。
  - 配置错误用 `oauthProxyErrorTransport` 返回明确错误，而不是静默直连。
- 各 provider（discord / github / generic / linuxdo / oidc / telegram）改用 `configuredOAuthHTTPClient`。
- 测试：`oauth/http_client_test.go`。

## 配置与开关

未设置 `OAUTH_PROXY_URL` 时全部直连；设置但 provider 未命中列表同样直连。

## 变更记录

| 日期 | 提交 | 变更 |
|---|---|---|
| 2026-09-23 | `cf0ef5fd7` | 初始引入 |

## 注意事项

- 涉及认证流程，改动须遵循 `AGENTS.md` 的 OWASP 要求；代理失败必须显式报错，不能回退到直连造成令牌泄露风险。
