# 03 · 部署与 CI（supervisor / Grafana / GHCR）

- **状态**：活跃
- **引入提交**：`3996b5642`（supervisor）、`3817d7dae`（GHCR CI）、`03d2ff99d`（CI 修复）
- **影响面**：部署目录、CI workflow

## 功能说明

补齐三种部署/运维能力：supervisor 进程托管配置、GHCR 镜像构建 CI、Grafana 看板（与功能 06 配套）。

## 实现要点

- `deploy/supervisor/new-api.conf`：supervisor program 定义（`/opt/new-api/start.sh`，user=newapi）。
- `deploy/supervisor/new-api.env.example`：160 行环境变量示例，含 `LOG_REQUEST_HEADERS`、`ERROR_LOG_ENABLED`、`RELAY_TTFT_TIMEOUT` 等。
- `.github/workflows/ghcr-build.yml`：构建并推送 GHCR 镜像；`03d2ff99d` 修复 upload-artifact 的 `VERSION` 变量未展开。
- Grafana 看板 JSON 见 `deploy/grafana/`（`newapi-overview.json`、`newapi-tenants.json`，随功能 06/07 引入）。
- `docker-compose.yml` 增加指标相关服务编排。

## 配置与开关

环境变量示例见 `deploy/supervisor/new-api.env.example`。线上实际生效值（截至 2026-10-27）：

| 变量 | 线上值 | 说明 |
|---|---|---|
| `LOG_REQUEST_HEADERS` | `true` | 打印 provider 响应头 |
| `ERROR_LOG_ENABLED` | `true` | 错误日志 |
| `RELAY_TTFT_TIMEOUT` | 60（默认 30） | 流式首包超时秒数 |

## 变更记录

| 日期 | 提交 | 变更 |
|---|---|---|
| 2026-10-21 | `3996b5642` | supervisor 配置与环境变量示例 |
| 2026-10-23 | `3817d7dae` | GHCR 镜像 workflow |
| 2026-10-23 | `03d2ff99d` | 修复 upload-artifact 路径 VERSION 未展开 |

## 注意事项

- 日志目录线上为 `/var/log/new-api/`，由 supervisor 轮转（`supervisor.log.N[.gz]`）。
