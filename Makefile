.PHONY: install dev server web test test-server test-web build mcp convert verify

# Local data lives next to the old v1 database so the first start converts it.
DATA ?= $(CURDIR)/finance_tracker/backend
PORT ?= 8080

install:
	cd finance_tracker/server && go mod download
	cd finance_tracker/web && npm ci

# API on :$(PORT) (first start converts $(DATA)/finance.db into finance-v2.db)
server:
	@if [ -f .env ]; then set -a && . ./.env && set +a; fi && \
	  cd finance_tracker/server && DB_PATH=$(DATA)/finance-v2.db V1_DB_PATH=$(DATA)/finance.db PORT=$(PORT) go run ./cmd/server

# Web app on :5175, proxying /api to the server
web:
	cd finance_tracker/web && API_URL=http://localhost:$(PORT) npx vite

dev:
	$(MAKE) -j2 server web

test: test-server test-web

test-server:
	cd finance_tracker/server && go vet ./... && go test -race ./...

test-web:
	cd finance_tracker/web && npx tsc --noEmit && npx vitest run

build:
	cd finance_tracker/server && go build -o bin/finance-tracker ./cmd/server && go build -o bin/finance-tracker-mcp ./cmd/mcp
	cd finance_tracker/web && npm run build

mcp:
	cd finance_tracker/server && go build -o bin/finance-tracker-mcp ./cmd/mcp

# Convert a v1 database or finances.json and print the reconciliation report.
convert:
	cd finance_tracker/server && go run ./cmd/v1convert -from $(FROM) -to $(TO)
