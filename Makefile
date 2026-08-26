.PHONY: run build test tidy compose-up compose-down compose-logs health infra-up infra-down infra-logs stream

run:
	go run ./cmd/pilot

build:
	go build -o bin/pilot ./cmd/pilot

test:
	go test ./...

tidy:
	go mod tidy

compose-up:
	docker compose -f manifest/docker-compose.yml up -d

compose-down:
	docker compose -f manifest/docker-compose.yml down

compose-logs:
	docker compose -f manifest/docker-compose.yml logs -f

health:
	@curl -sf http://localhost:8080/api/v1/health/live && echo "live ok"
	@curl -s http://localhost:8080/api/v1/health/ready

# 本地基础设施：复用本机已 pull 的镜像，不依赖 compose（避免与运行中的 redis 冲突）。
# Redis 复用已在 6379 运行的 redis-stack-server，不由本目标重复起。
infra-up:
	@echo ">> 准备本地 MySQL(3306) 与 Elasticsearch(9200)"
	@docker rm -f pilot-mysql 2>/dev/null || true
	@docker rm -f pilot-es 2>/dev/null || true
	docker run -d --name pilot-mysql \
		-e MYSQL_ROOT_PASSWORD=root \
		-e MYSQL_DATABASE=pilot \
		-e MYSQL_USER=pilot \
		-e MYSQL_PASSWORD=pilot \
		-p 3306:3306 \
		-v "$(CURDIR)/manifest/mysql/init:/docker-entrypoint-initdb.d:ro" \
		mysql:8.0 \
		--character-set-server=utf8mb4 --collation-server=utf8mb4_unicode_ci
	docker run -d --name pilot-es \
		-e discovery.type=single-node -e xpack.security.enabled=false \
		-e ES_JAVA_OPTS="-Xms512m -Xmx512m" \
		-p 9200:9200 \
		elasticsearch:8.18.8
	@echo ">> 等待 Elasticsearch 就绪(最多 120s)..."
	@for i in $$(seq 1 120); do \
		status=$$(curl -sf http://127.0.0.1:9200/_cluster/health 2>/dev/null | grep -o '"status":"[a-z]*"' | head -1 | cut -d'"' -f4); \
		if [ -n "$$status" ] && [ "$$status" != "red" ]; then \
			echo ">> Elasticsearch 就绪(status=$$status)"; echo ">> MySQL 与 Elasticsearch 已启动"; exit 0; \
		fi; \
		sleep 1; \
	done; \
	echo ">> Elasticsearch 未在 120s 内就绪"; docker logs pilot-es; exit 1

infra-down:
	docker rm -f pilot-mysql pilot-es 2>/dev/null || true

infra-logs:
	docker logs -f pilot-mysql pilot-es

stream:
	@curl -N http://localhost:8080/api/v1/chat/stream \
		-H "Content-Type: application/json" \
		-d '{"user_id":"u1","session_id":"s1","query":"hello"}'
