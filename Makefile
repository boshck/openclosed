.PHONY: help test check-migrations build up down migrate push pull deploy-tag logs ps backup allowlist-add allowlist-list require-env require-dockerhub-user require-clean-worktree require-allowlist-args

GREEN := \033[0;32m
YELLOW := \033[0;33m
RED := \033[0;31m
NC := \033[0m

ifeq ($(shell command -v docker-compose >/dev/null 2>&1; echo $$?),0)
	DOCKER_COMPOSE = docker-compose
else
	DOCKER_COMPOSE = docker compose
endif

ENV ?= local
service ?=
GO_CACHE_DIR ?= /tmp/openclosed-go-cache
GO_MOD_CACHE_DIR ?= /tmp/openclosed-go-mod-cache
DOCKERHUB_USER ?= $(shell [ -f .env ] && sed -n 's/^DOCKERHUB_USER=//p' .env | tail -n 1 | tr -d '\r')
IMAGE_TAG ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo local)
BOT_IMAGE ?= $(DOCKERHUB_USER)/openclosed-bot
DEPLOY_SERVICES := bot
RUNTIME_SERVICES := postgres migrate bot
note ?= manual

help:
	@printf '%b\n' "$(GREEN)openclosed commands$(NC)"
	@printf '%s\n' "  make test                         - check migrations and run Go tests"
	@printf '%s\n' "  make check-migrations             - validate SQL migration up/down pairs"
	@printf '%s\n' "  make build                        - build Docker image"
	@printf '%s\n' "  make up ENV=local                 - start local postgres, migrate, bot"
	@printf '%s\n' "  make down ENV=local               - stop local compose project"
	@printf '%s\n' "  make migrate ENV=local            - run PostgreSQL migration service"
	@printf '%s\n' "  make push                         - push Docker image with current tag and latest"
	@printf '%s\n' "  make pull ENV=staging             - server deploy from current git commit"
	@printf '%s\n' "  make deploy-tag ENV=prod tag=<sha|latest>"
	@printf '%s\n' "  make logs ENV=local service=bot"
	@printf '%s\n' "  make ps ENV=prod"
	@printf '%s\n' "  make backup ENV=prod"
	@printf '%s\n' "  make allowlist-add chat_id=<id> user_id=<id> note=<text>"
	@printf '%s\n' "  make allowlist-list ENV=local"

test: check-migrations
	GOCACHE="$(GO_CACHE_DIR)" GOMODCACHE="$(GO_MOD_CACHE_DIR)" go test -v ./...

check-migrations:
	@./scripts/check_migrations.sh

build: check-migrations
	@printf '%b\n' "$(GREEN)Building Docker image with tag $(IMAGE_TAG)...$(NC)"
	DOCKERHUB_USER="$(DOCKERHUB_USER)" IMAGE_TAG="$(IMAGE_TAG)" $(DOCKER_COMPOSE) build $(DEPLOY_SERVICES)

up: require-env check-migrations
	@printf '%b\n' "$(GREEN)Starting openclosed $(ENV)...$(NC)"
	COMPOSE_PROJECT_NAME="openclosed_$(ENV)" $(DOCKER_COMPOSE) up -d --build $(RUNTIME_SERVICES)

down: require-env
	COMPOSE_PROJECT_NAME="openclosed_$(ENV)" $(DOCKER_COMPOSE) down

migrate: require-env check-migrations
	COMPOSE_PROJECT_NAME="openclosed_$(ENV)" $(DOCKER_COMPOSE) run --rm migrate

push: require-clean-worktree require-dockerhub-user build
	@set -e; \
	printf '%b\n' "$(YELLOW)Pushing $(BOT_IMAGE):$(IMAGE_TAG) and $(BOT_IMAGE):latest$(NC)"; \
	docker push "$(BOT_IMAGE):$(IMAGE_TAG)"; \
	docker tag "$(BOT_IMAGE):$(IMAGE_TAG)" "$(BOT_IMAGE):latest"; \
	docker push "$(BOT_IMAGE):latest"

pull: require-env require-dockerhub-user
	@set -e; \
	git pull --ff-only; \
	TAG="$$(git rev-parse --short HEAD)"; \
	printf '%b\n' "$(YELLOW)Deploying $(ENV) tag $$TAG...$(NC)"; \
	DOCKERHUB_USER="$(DOCKERHUB_USER)" IMAGE_TAG="$$TAG" COMPOSE_PROJECT_NAME="openclosed_$(ENV)" $(DOCKER_COMPOSE) pull $(DEPLOY_SERVICES); \
	DOCKERHUB_USER="$(DOCKERHUB_USER)" IMAGE_TAG="$$TAG" COMPOSE_PROJECT_NAME="openclosed_$(ENV)" $(DOCKER_COMPOSE) up -d --no-build $(RUNTIME_SERVICES); \
	DOCKERHUB_USER="$(DOCKERHUB_USER)" IMAGE_TAG="$$TAG" COMPOSE_PROJECT_NAME="openclosed_$(ENV)" $(DOCKER_COMPOSE) ps

deploy-tag: require-env require-dockerhub-user
ifndef tag
	$(error $(RED)Use: make deploy-tag ENV=prod tag=<sha|latest>$(NC))
endif
	@set -e; \
	printf '%b\n' "$(YELLOW)Deploying $(ENV) tag $(tag)...$(NC)"; \
	DOCKERHUB_USER="$(DOCKERHUB_USER)" IMAGE_TAG="$(tag)" COMPOSE_PROJECT_NAME="openclosed_$(ENV)" $(DOCKER_COMPOSE) pull $(DEPLOY_SERVICES); \
	DOCKERHUB_USER="$(DOCKERHUB_USER)" IMAGE_TAG="$(tag)" COMPOSE_PROJECT_NAME="openclosed_$(ENV)" $(DOCKER_COMPOSE) up -d --no-build $(RUNTIME_SERVICES); \
	DOCKERHUB_USER="$(DOCKERHUB_USER)" IMAGE_TAG="$(tag)" COMPOSE_PROJECT_NAME="openclosed_$(ENV)" $(DOCKER_COMPOSE) ps

logs: require-env
	@if [ -n "$(service)" ]; then \
		COMPOSE_PROJECT_NAME="openclosed_$(ENV)" $(DOCKER_COMPOSE) logs -f "$(service)"; \
	else \
		COMPOSE_PROJECT_NAME="openclosed_$(ENV)" $(DOCKER_COMPOSE) logs -f; \
	fi

ps: require-env
	@COMPOSE_PROJECT_NAME="openclosed_$(ENV)" $(DOCKER_COMPOSE) ps

backup: require-env
	@if [ "$(ENV)" != "prod" ]; then \
		printf '%b\n' "$(RED)backup is allowed only with ENV=prod$(NC)"; \
		exit 1; \
	fi
	@mkdir -p backups/postgres
	@backup="backups/postgres/openclosed_$$(date -u +%Y%m%dT%H%M%SZ).sql.gz"; \
	printf '%b\n' "$(YELLOW)Writing $$backup$(NC)"; \
	COMPOSE_PROJECT_NAME="openclosed_$(ENV)" $(DOCKER_COMPOSE) exec -T postgres sh -c 'pg_dump -U "$$POSTGRES_USER" "$$POSTGRES_DB"' | gzip > "$$backup"; \
	test -s "$$backup"; \
	find backups/postgres -type f -name '*.sql.gz' -mtime +14 -delete; \
	printf '%b\n' "$(GREEN)Backup complete: $$backup$(NC)"

allowlist-add: require-env require-allowlist-args
	@COMPOSE_PROJECT_NAME="openclosed_$(ENV)" $(DOCKER_COMPOSE) exec -T postgres sh -c 'psql -U "$$POSTGRES_USER" -d "$$POSTGRES_DB" -v chat_id="$$1" -v user_id="$$2" -v note="$$3" -c "INSERT INTO allowlist (chat_id, user_id, note) VALUES (:chat_id::bigint, :user_id::bigint, :'\''note'\'') ON CONFLICT (chat_id, user_id) DO UPDATE SET note = excluded.note;"' sh "$(chat_id)" "$(user_id)" "$(note)"

allowlist-list: require-env
	@COMPOSE_PROJECT_NAME="openclosed_$(ENV)" $(DOCKER_COMPOSE) exec -T postgres sh -c 'psql -U "$$POSTGRES_USER" -d "$$POSTGRES_DB" -c "SELECT chat_id, user_id, note, created_at FROM allowlist ORDER BY chat_id, user_id;"'

require-env:
	@if [ "$(ENV)" != "staging" ] && [ "$(ENV)" != "prod" ] && [ "$(ENV)" != "local" ]; then \
		printf '%b\n' "$(RED)ENV must be local, staging, or prod$(NC)"; \
		exit 1; \
	fi

require-dockerhub-user:
	@if [ -z "$(DOCKERHUB_USER)" ]; then \
		printf '%b\n' "$(RED)Set DOCKERHUB_USER in .env or pass DOCKERHUB_USER=<name>$(NC)"; \
		exit 1; \
	fi

require-clean-worktree:
	@if [ -n "$$(git status --porcelain)" ]; then \
		printf '%b\n' "$(RED)Commit or stash local changes before make push; Docker image tag is based on git HEAD.$(NC)"; \
		exit 1; \
	fi

require-allowlist-args:
ifndef chat_id
	$(error $(RED)Use: make allowlist-add chat_id=<telegram_chat_id> user_id=<telegram_user_id>$(NC))
endif
ifndef user_id
	$(error $(RED)Use: make allowlist-add chat_id=<telegram_chat_id> user_id=<telegram_user_id>$(NC))
endif
