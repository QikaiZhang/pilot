.PHONY: run build test tidy compose-up compose-down compose-logs health

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
