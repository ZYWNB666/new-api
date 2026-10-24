# new-api 部署指南（Supervisord + 阿里云 PG/Redis + ALB）

## 文件说明

| 文件 | 用途 | 部署位置 |
|---|---|---|
| `new-api.conf` | Supervisord 进程配置 | `/etc/supervisor/conf.d/new-api.conf` |
| `new-api.env.example` | 环境变量模板 | `/usr/local/new-api/.env` |

## 前置条件

- 服务器已安装 `supervisor`、`bash`
- 已编译 `new-api` 二进制（`GOWORK=off go build -o new-api .`）
- 阿里云 PostgreSQL 实例已创建，数据库已建
- 阿里云 Redis 实例已创建

## 获取二进制

### 方式一：GitHub Actions 自动构建（推荐）

推送到 `main` 或 `codex-fork` 分支后，GitHub Actions 会自动构建：

1. 进入仓库 **Actions** 页面
2. 找到 **Build & Push Docker Image to GHCR** 工作流
3. 下载 **new-api-binary** artifact（含 `new-api-<version>-linux-amd64` 和 `.sha256`）

### 方式二：手动编译

```bash
# 构建前端
cd web && bun install --frozen-lockfile && bun run build && cd ..

# 编译二进制（嵌入前端）
GOWORK=off go build -ldflags "-s -w -X 'github.com/QuantumNous/new-api/common.Version=$(cat VERSION)'" -o new-api .
```

## 部署步骤

```bash
# 1. 创建目录
sudo mkdir -p /usr/local/new-api
sudo mkdir -p /var/log/new-api

# 2. 上传二进制
sudo cp new-api /usr/local/new-api/

# 3. 配置环境变量（.env 放工作目录，程序自动加载）
sudo cp deploy/supervisor/new-api.env.example /usr/local/new-api/.env
sudo vim /usr/local/new-api/.env   # 填写 PG、Redis、密钥

# 4. 配置 supervisord
sudo cp deploy/supervisor/new-api.conf /etc/supervisor/conf.d/
sudo supervisorctl reread
sudo supervisorctl update

# 5. 启动
sudo supervisorctl start new-api

# 6. 查看状态
sudo supervisorctl status new-api
sudo tail -f /var/log/new-api/app.log
```

## 阿里云配置要点

### PostgreSQL DSN 格式
```
postgres://<用户名>:<密码>@<实例地址>:<端口>/<数据库名>?sslmode=disable
```
- 阿里云 PG 默认要求 SSL，如连接报错，将 `sslmode=disable` 改为 `sslmode=require`
- 实例地址在阿里云控制台「连接信息」中查看（内网地址更快）

### Redis 连接格式
```
redis://:<密码>@<实例地址>:<端口>/0
```
- 阿里云 Redis 通常无用户名，密码前加冒号
- 实例地址在阿里云控制台「连接信息」中查看

### 会话密钥生成
```bash
openssl rand -hex 32
```

## ALB 配置要点

| 配置项 | 值 |
|---|---|
| 监听协议 | HTTP |
| 监听端口 | 80 / 443 |
| 后端协议 | HTTP |
| 后端端口 | 3000 |
| 健康检查路径 | `/api/status` |
| 健康检查响应 | `200` |
| 会话保持 | 建议开启（基于 Cookie） |
| 超时时间 | 建议 600s（AI 请求可能较慢） |

## 常用运维命令

```bash
# 重启
sudo supervisorctl restart new-api

# 停止
sudo supervisorctl stop new-api

# 查看日志
sudo tail -f /var/log/new-api/app.log
sudo tail -f /var/log/new-api/error.log

# 查看进程
ps aux | grep new-api
```

## 用户与模型流量监控

启用 Prometheus metrics 后，`/metrics` 提供以下带 `tenant_id`（用户账号 ID）、`model`（客户端原始模型名）和 `scope` 标签的指标：

- `new_api_generation_started_total`：进入 relay 的请求数，包含本地限流拒绝。
- `new_api_generation_tokens_total{direction="input|output"}`：完成请求的输入、输出 token，包含缓存输入和适配器估算；缺失、无效输入用量不计入。
- `new_api_generation_requests_in_flight`：完整请求生命周期的当前并发，包括等待上游、流式生成；结束、取消或异常时释放。
- `new_api_generation_requests_total{status="...",outcome="..."}`：完成请求的 HTTP 状态码和协议结果。流式响应提交后失败仍可能是 HTTP 200。

使用 `scope="site"` 时每个客户端请求计一次；`scope="provider"` 按渠道尝试计数。管理接口不计入生成并发。验证请求体之前的拒绝无法归属到模型，不计入这些生成指标。超出 `METRICS_MAX_GENERATION_LABELSETS` 的新标签组合聚合到 `__other__`。

将 `deploy/prometheus/newapi-traffic.rules.yml` 加入 Prometheus 的 `rule_files` 并重新加载配置，提供 `new_api_user_model_rpm`、`new_api_user_model_tpm`、`new_api_user_model_running_requests`。规则要求采集任务名为 `new-api`，建议每 15 秒采样。

```yaml
rule_files:
  - /opt/monitoring/config/rules/newapi-traffic.rules.yml
```

导入 `deploy/grafana/newapi-traffic.json`，数据源 UID 为 `newapi-prometheus`。面板支持实例、用户 ID、模型筛选，展示 RPM、TPM、并发、状态码和用量质量。RPM/TPM 使用最近 1 分钟速率乘 60，不是限流令牌桶的占用或剩余额度；TPM 在请求完成后更新，短请求可能无法被并发采样捕获，新指标不回填历史数据。

规则可使用 `promtool test rules deploy/prometheus/newapi-traffic.test.yml` 验证。
