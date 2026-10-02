DOCKER_COMPOSE ?= docker compose
GO_IMAGE ?= golang:1.26.6-alpine3.23
APP_NETWORK ?= go-recorder_app-network

.PHONY: serve api worker media-worker product-worker migrate doctor test up restart migrate-up test-run debug-api debug-worker debug-both debug-stop docker-build docker-up docker-down docker-restart

serve:
	go run ./cmd/main serve

api:
	go run ./cmd/api

worker:
	go run ./cmd/worker

media-worker:
	go run ./cmd/media-worker

product-worker:
	go run ./cmd/product-worker

migrate:
	$(MAKE) migrate-up

doctor:
	$(DOCKER_COMPOSE) ps
	$(DOCKER_COMPOSE) exec -T rabbitmq rabbitmq-diagnostics -q check_port_connectivity
	$(DOCKER_COMPOSE) exec -T api wget -qO- http://127.0.0.1:8085/health
	$(DOCKER_COMPOSE) exec -T worker wget -qO- http://127.0.0.1:8090/health

test:
	$(MAKE) test-run

up:
	$(DOCKER_COMPOSE) up -d --build --remove-orphans

restart:
	$(DOCKER_COMPOSE) down
	$(DOCKER_COMPOSE) up -d --build --remove-orphans

migrate-up:
	docker run --rm \
		--network $(APP_NETWORK) \
		--env-file .env \
		-v "$(CURDIR):/src" \
		-w /src \
		$(GO_IMAGE) go run ./cmd/main migrate

test-run:
	docker run --rm \
		-v "$(CURDIR):/src" \
		-w /src \
		$(GO_IMAGE) go test ./...

debug-api:
	$(DOCKER_COMPOSE) stop api
	$(DOCKER_COMPOSE) --profile debug up -d --build api-debug

debug-worker:
	$(DOCKER_COMPOSE) stop worker
	$(DOCKER_COMPOSE) --profile debug up -d --build worker-debug

debug-both:
	$(DOCKER_COMPOSE) stop api worker
	$(DOCKER_COMPOSE) --profile debug up -d --build api-debug worker-debug

debug-stop:
	$(DOCKER_COMPOSE) stop api-debug worker-debug

docker-build:
	$(DOCKER_COMPOSE) build api worker media-worker product-worker frontend minio

docker-up:
	$(MAKE) up

docker-down:
	$(DOCKER_COMPOSE) down

docker-restart:
	$(MAKE) restart

.PHONY: stage6-up stage6-failure stage6-load stage6-soak
stage6-up:
	$(DOCKER_COMPOSE) -p recorder-stage6 -f docker-compose.stage6.yml up -d --build
stage6-failure:
	RECORDER_STAGE6_FAILURE=true go run ./tools/stage_six_failure
stage6-load:
	go run ./tools/stage_six_load
	RECORDER_MEDIA_LOAD=true go test -v ./internal/infrastructure/sfu -run '^TestStageSixMediaLoad$$' -timeout 5m
stage6-soak:
	RECORDER_SOAK_DURATION=30m go test -v ./internal/infrastructure/sfu -run '^TestStageSixSoak$$' -timeout 35m
