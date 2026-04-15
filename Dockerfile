# Stage 1 — build Go backend
FROM golang:1.22-alpine AS backend-builder
WORKDIR /app/backend
RUN apk add --no-cache gcc musl-dev
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
RUN go build -o finance-tracker ./cmd/api/main.go

# Stage 2 — build React frontend
FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ .
RUN npm run build

# Stage 3 — final image: nginx + Go binary in one container
FROM nginx:stable-alpine
WORKDIR /app

RUN apk add --no-cache ca-certificates jq openssl

# Go binary
COPY --from=backend-builder /app/backend/finance-tracker ./finance-tracker

# React static files
COPY --from=frontend-builder /app/frontend/dist /usr/share/nginx/html

# nginx config (proxies /api/ to localhost:8080)
COPY nginx.conf /etc/nginx/conf.d/default.conf

# startup script
COPY run.sh /run.sh
RUN chmod +x /run.sh /app/finance-tracker

EXPOSE 80

CMD ["/run.sh"]
