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
