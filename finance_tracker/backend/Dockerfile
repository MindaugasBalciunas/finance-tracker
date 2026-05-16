FROM golang:1.22-alpine AS builder
WORKDIR /app
RUN apk add --no-cache gcc musl-dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o finance-tracker ./cmd/api/main.go

FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/finance-tracker .

EXPOSE 8080
ENV DB_PATH=/data/finance.db
VOLUME ["/data"]

CMD ["./finance-tracker"]
