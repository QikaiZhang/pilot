# 环境与启动规格

## 前置软件

- Go 1.24 或更高版本
- Docker Engine 与 Docker Compose v2
- `curl`；Stage 09 额外需要 `protoc`、Go protobuf/gRPC 插件和 `grpcurl`

## 固定端口

| 服务 | 默认端口 | 用途 |
| --- | ---: | --- |
| Pilot HTTP | 8080 | API、SSE、静态页面 |
| Redis | 6379 | 短期记忆 |
| MySQL | 3306 | 长期历史 |
| Elasticsearch | 9200 | 知识库和运行日志 |
| Prometheus | 9090 | 指标查询 |
| Jaeger UI | 16686 | Trace 查询 |
| Jaeger OTLP gRPC | 4317 | Trace 上报 |

## `.env.example` 最低字段

```dotenv
APP_ADDR=:8080
APP_ENV=local
ALLOW_DEGRADED=true

REDIS_ADDR=127.0.0.1:6379
REDIS_PASSWORD=
REDIS_DB=0
REDIS_MEMORY_TTL=24h
REDIS_MAX_MESSAGES=20

MYSQL_DSN=pilot:pilot@tcp(127.0.0.1:3306)/pilot?parseTime=true&charset=utf8mb4

ES_URL=http://127.0.0.1:9200
ES_USERNAME=
ES_PASSWORD=
ES_KNOWLEDGE_INDEX=pilot_knowledge
ES_LOG_INDEX=pilot_logs

LLM_MODE=mock
LLM_BASE_URL=
LLM_API_KEY=
LLM_MODEL=
EMBEDDING_MODE=mock
EMBEDDING_DIM=32

API_KEY=
RATE_LIMIT_REQUESTS=60
RATE_LIMIT_WINDOW=1m
OTEL_EXPORTER_OTLP_ENDPOINT=127.0.0.1:4317
```

真实密钥只放本地 `.env`，不得提交；`LLM_MODE=mock` 是默认值，保证没有供应商账号也能完成 Stage 01-03 和测试。

## 启动顺序

1. `docker compose up -d redis mysql elasticsearch prometheus jaeger`
2. 等待 `/api/v1/health/ready` 返回 `200`；如果 `ALLOW_DEGRADED=true`，只有观测依赖允许延迟。
3. `go run ./cmd/pilot`
4. `curl -i http://localhost:8080/api/v1/health/live`
5. `curl -i http://localhost:8080/api/v1/health/ready`

预期：live 只证明进程存在；ready 逐项报告 Redis、MySQL、ES 的状态。应用启动失败时，错误中必须包含依赖名，但不得包含密码或完整 DSN。

## 停止与清理

```bash
docker compose down
docker compose down -v  # 仅在确认要删除本地数据库卷时使用
```

阶段测试使用 `pilot_test_<随机后缀>` 用户、会话和 ES 文档前缀，避免清理其他数据。
