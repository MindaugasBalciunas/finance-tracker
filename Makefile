.PHONY: backend frontend install test test-backend test-frontend swagger dev

# Install all dependencies
install:
	cd backend && go mod tidy
	cd frontend && npm install

# Generate Swagger docs (requires swag: go install github.com/swaggo/swag/cmd/swag@latest)
swagger:
	cd backend && swag init -g cmd/api/main.go --output docs

AIR := $(shell which air 2>/dev/null || echo $(HOME)/go/bin/air)

# Run backend (with live reload via air)
# Loads .env from project root if present
backend:
	@lsof -ti:8080 | xargs kill -9 2>/dev/null || true
	@if [ -f .env ]; then set -a && . ./.env && set +a; fi && \
	  cd backend && $(AIR)

# Run backend without live reload (no air needed)
run-backend:
	@if [ -f .env ]; then set -a && . ./.env && set +a; fi && \
	  cd backend && go run ./cmd/api/main.go

# Run frontend dev server
frontend:
	cd frontend && npm run dev

# Run both concurrently (requires 'make -j2')
dev:
	$(MAKE) -j2 backend frontend

# Run all tests
test: test-backend test-frontend

# Run Go tests
test-backend:
	cd backend && go test ./... -v -cover

# Run React tests
test-frontend:
	cd frontend && npm test

# Build frontend for production
build-frontend:
	cd frontend && npm run build

# Build backend binary
build-backend:
	cd backend && go build -o bin/finance-tracker ./cmd/api/main.go

# Build everything
build: swagger build-backend build-frontend

# Docker (future)
docker-build:
	docker build -t finance-tracker-backend ./backend
	docker build -t finance-tracker-frontend ./frontend
