# QB Notify v2 — tareas de desarrollo.
# Requiere Go en el PATH (instalado en ~/sdk/go/bin) y Docker + Compose.

GO       ?= go
BACKEND  := backend
# Usa el plugin `docker compose` si está; si no, el binario standalone.
COMPOSE  ?= $(shell docker compose version >/dev/null 2>&1 && echo "docker compose" || echo "docker-compose") -f deploy/docker-compose.yml

# Carga .env (si existe) y exporta sus variables para los targets de ejecución.
ifneq (,$(wildcard .env))
include .env
export
endif

.PHONY: help up down logs ps run-api run-worker build test lint fmt tidy vet smoke migrate-up migrate-down migrate-status seed sqlc reencrypt

help: ## Muestra esta ayuda
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

up: ## Levanta la infraestructura local (Postgres + Redis Sentinel + MailHog)
	$(COMPOSE) up -d

down: ## Detiene la infraestructura local
	$(COMPOSE) down

logs: ## Muestra logs de la infraestructura
	$(COMPOSE) logs -f

ps: ## Estado de los contenedores
	$(COMPOSE) ps

run-api: ## Ejecuta el servidor API en el host
	cd $(BACKEND) && $(GO) run ./cmd/api

run-worker: ## Ejecuta el worker en el host
	cd $(BACKEND) && $(GO) run ./cmd/worker

build: ## Compila ambos binarios en ./bin
	cd $(BACKEND) && $(GO) build -o ../bin/api ./cmd/api
	cd $(BACKEND) && $(GO) build -o ../bin/worker ./cmd/worker

test: ## Ejecuta los tests
	cd $(BACKEND) && $(GO) test ./...

vet: ## go vet
	cd $(BACKEND) && $(GO) vet ./...

fmt: ## Formatea el código
	cd $(BACKEND) && $(GO) fmt ./...

tidy: ## Ordena go.mod/go.sum
	cd $(BACKEND) && $(GO) mod tidy

lint: ## Linter (requiere golangci-lint)
	cd $(BACKEND) && golangci-lint run

migrate-up: ## Aplica migraciones pendientes
	cd $(BACKEND) && $(GO) run ./cmd/migrate up

migrate-down: ## Revierte la última migración
	cd $(BACKEND) && $(GO) run ./cmd/migrate down

migrate-status: ## Estado de las migraciones
	cd $(BACKEND) && $(GO) run ./cmd/migrate status

seed: ## Siembra datos iniciales (base templates, super-admin, app demo)
	cd $(BACKEND) && $(GO) run ./cmd/seed

reencrypt: ## Re-cifra los secretos a la versión de clave primaria (rotación KMS)
	cd $(BACKEND) && $(GO) run ./cmd/reencrypt

sqlc: ## Regenera el código sqlc (vía Docker)
	docker run --rm --user "$$(id -u):$$(id -g)" -v "$(PWD)/$(BACKEND):/src" -w /src sqlc/sqlc:latest generate

smoke: ## Levanta infra + API y verifica /readyz (verificación de Fase 0)
	./deploy/smoke.sh
