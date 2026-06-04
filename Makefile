# MediaForge — developer task runner
.DEFAULT_GOAL := help
SHELL := /bin/bash

.PHONY: help up down logs ps build test lint fmt smoke clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
	  awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

up: ## Start the full stack (build + run)
	docker compose up --build -d

down: ## Stop the stack and remove containers
	docker compose down

logs: ## Tail logs from all services
	docker compose logs -f --tail=100

ps: ## Show running services
	docker compose ps

build: ## Build all images without starting
	docker compose build

test: ## Run unit tests across all services
	cd services/api-gateway && go test ./...
	cd services/worker-image && go test ./...
	cd services/worker-ocr && python -m pytest -q
	cd services/realtime-gateway && npm test --silent

lint: ## Lint all services
	cd services/api-gateway && go vet ./...
	cd services/worker-image && go vet ./...
	cd services/worker-ocr && ruff check .
	cd services/realtime-gateway && npm run lint --silent

fmt: ## Format all services
	cd services/api-gateway && gofmt -w .
	cd services/worker-image && gofmt -w .
	cd services/worker-ocr && ruff format .
	cd services/realtime-gateway && npm run format --silent

smoke: ## End-to-end smoke test against a running stack
	./scripts/smoke.sh

clean: ## Remove containers, volumes and build artifacts
	docker compose down -v
	rm -rf services/*/bin services/*/dist
