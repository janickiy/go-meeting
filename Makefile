DOCKER_COMPOSE ?= docker compose
GO_IMAGE ?= golang:1.26.9-alpine3.23
APP_NETWORK ?= go-recorder_app-network

.PHONY: serve api worker media-worker product-worker live-worker migrate doctor up restart migrate-up debug-api debug-worker debug-both debug-stop docker-build docker-up docker-down docker-restart

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

live-worker:
	go run ./cmd/live-worker

migrate:
	$(MAKE) migrate-up

doctor:
	$(DOCKER_COMPOSE) ps
	$(DOCKER_COMPOSE) exec -T rabbitmq rabbitmq-diagnostics -q check_port_connectivity
	$(DOCKER_COMPOSE) exec -T api wget -qO- http://127.0.0.1:8085/health
	$(DOCKER_COMPOSE) exec -T worker wget -qO- http://127.0.0.1:8090/health

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
	$(DOCKER_COMPOSE) build api worker media-worker product-worker live-worker frontend minio

docker-up:
	$(MAKE) up

docker-down:
	$(DOCKER_COMPOSE) down

docker-restart:
	$(MAKE) restart

# RELEASE_ARGS содержит явные --environment, --env-file, --manifest и --project.
.PHONY: release-validate release-pull release-migrate release-drain release-resume release-deploy release-rollback release-build release-package
release-validate:
	bash scripts/release/release.sh validate $(RELEASE_ARGS)
release-pull:
	bash scripts/release/release.sh pull $(RELEASE_ARGS)
release-migrate:
	bash scripts/release/release.sh migrate $(RELEASE_ARGS)
release-drain:
	bash scripts/release/release.sh drain $(RELEASE_ARGS)
release-resume:
	bash scripts/release/release.sh resume $(RELEASE_ARGS)
release-deploy:
	bash scripts/release/release.sh deploy $(RELEASE_ARGS)
release-rollback:
	bash scripts/release/release.sh rollback $(RELEASE_ARGS)
release-build:
	bash scripts/release/build.sh $(BUILD_ARGS)
release-package:
	bash scripts/release/package.sh $(PACKAGE_ARGS)
