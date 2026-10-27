# 06 · 可观测性指标（Prometheus / Grafana）

- **状态**：活跃
- **引入提交**：`10483148d`（HTTP/Redis/DB 指标）、`c52ce1ac7`（生成类指标）
- **影响面**：`pkg/metrics`、`relay/common/relay_info.go`、Grafana 看板

## 功能说明

在既有 `pkg/perf_metrics`（内部采样落库）之外，新增面向 Prometheus 的指标导出与 Grafana 看板。

## 实现要点

- `pkg/metrics/`：
  - `metrics.go`：指标注册与采集入口。
  - `generation.go`：按请求/模型的生成类指标（请求数、token 用量等）。
  - `content.go`、`pools.go`、`upstream.go`：内容长度、连接池、上游维度指标。
- `relay/common/relay_info.go`：补充指标所需字段。
- `deploy/grafana/newapi-overview.json`（6555 行）、`newapi-tenants.json`（1937 行）。
- 前端 `web/src/features/home/hooks/use-home-api-url.ts` 读取指标摘要用于首页展示。

## 配置与开关

Prometheus 抓取 `/metrics`（或部署配置的指标端点）。Grafana 看板从 `deploy/grafana/` 导入。

## 变更记录

| 日期 | 提交 | 变更 |
|---|---|---|
| 2026-10-22 | `10483148d` | HTTP/Redis/DB 综合指标 |
| 2026-10-24 | `c52ce1ac7` | 生成类指标（请求与 token） |

## 注意事项

- `pkg/metrics`（Prometheus 导出）与 `pkg/perf_metrics`（内部落库采样）是两套体系，改动时不要混淆。
- 指标标签避免高基数（如把用户 ID 当标签），否则 Prometheus 内存会爆。
