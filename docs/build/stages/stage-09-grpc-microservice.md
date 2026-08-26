# Stage 09：gRPC 与微服务打通练习

## 目标

在单体闭环已经通过 Stage 07 后，把“知识检索”拆成一个独立进程，完整体验 protobuf、gRPC server/client、HTTP 网关、超时、错误映射、Docker Compose 和 Trace 透传。这个阶段是练习拆分，不是为了让最终项目变复杂。

## 服务边界

```text
HTTP Controller -> KnowledgeClient -> KnowledgeService(gRPC) -> Elasticsearch/Embedder
```

HTTP 层继续负责鉴权、请求校验和对外错误码；gRPC 服务负责检索、索引和 ES 错误转换；ES 连接信息只出现在 gRPC 服务配置中。

## protobuf 最低契约

```proto
service KnowledgeService {
  rpc Search(SearchRequest) returns (SearchResponse);
  rpc Upload(UploadRequest) returns (UploadResponse);
}
message SearchRequest { string query = 1; string category = 2; int32 top_k = 3; string mode = 4; }
message SearchResponse { repeated Hit hits = 1; repeated string limitations = 2; }
message Hit { string doc_id = 1; string chunk_id = 2; string title = 3; string content = 4; float score = 5; }
```

## 实现顺序

1. 先在单体中写 `KnowledgeClient` 接口和 fake client，保证 Controller 测试不依赖网络。
2. 生成 protobuf Go 代码；手写 gRPC server 的参数校验、超时和错误转换。
3. 手写 client：为每次调用设置 deadline，并把 HTTP Trace-ID 放入 gRPC metadata。
4. 将单体 Controller 的检索调用替换为 client，保留同一 HTTP API。
5. Compose 增加 `knowledge-service`，验证 HTTP -> gRPC -> ES 链路。
6. 关闭 gRPC 服务，确认网关返回 `DEPENDENCY_UNAVAILABLE`；仅对幂等 Search 做一次有限重试。

## 手写/AI 边界

- S：protobuf 字段取舍、deadline、metadata、错误映射、重试边界。
- A：server/client 骨架和健康检查注册。
- B：生成代码、Compose 服务模板和重复 DTO。

## 验收

- `grpcurl` 能直接调用 Search，并得到稳定响应。
- HTTP API 的响应格式在拆分前后不变。
- Trace-ID 能从 HTTP 进入 gRPC，并在服务日志中出现。
- gRPC 超时、不可用、无数据三种情况能分别解释。
- 至少有一个跨进程 E2E 测试和一个 client 单元测试。

## 不做的事情

本阶段不引入服务注册中心、消息队列、Kubernetes 或多套数据库。先把一次 RPC 调通，再讨论扩展。
