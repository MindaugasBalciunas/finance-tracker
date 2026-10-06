# Stage 1 — Go server (cross-compiled on the build host)
FROM --platform=$BUILDPLATFORM golang:1.26.8-alpine AS server
ARG TARGETARCH
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ .
COPY config.yaml /config.yaml
RUN VERSION=$(sed -n 's/^version: *"\(.*\)"/\1/p' /config.yaml) \
 && CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o /out/finance-tracker ./cmd/server \
 && CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w" -o /out/finance-tracker-mcp ./cmd/mcp

# Stage 2 — web app (arch-independent output)
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /src
COPY web/package*.json ./
RUN npm ci
COPY web/ .
RUN npm run build

# Stage 3 — nginx + server in one container
FROM nginx:stable-alpine
WORKDIR /app
RUN apk add --no-cache ca-certificates jq openssl tzdata
COPY --from=server /out/finance-tracker /out/finance-tracker-mcp ./
COPY --from=web /src/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY run.sh /run.sh
RUN chmod +x /run.sh /app/finance-tracker /app/finance-tracker-mcp
# Fail the build on a broken nginx config — checked with the password prompt
# on, the way run.sh writes it (auth.conf is regenerated at start-up).
RUN printf 'satisfy any;\nauth_basic "Finance Tracker";\nauth_basic_user_file /etc/nginx/.htpasswd;\nauth_request /_gate;\n' > /etc/nginx/auth.conf \
 && printf 'auth_request /_gate_open;\n' > /etc/nginx/auth-open.conf \
 && touch /etc/nginx/.htpasswd && nginx -t \
 && printf '# auth disabled\n' > /etc/nginx/auth.conf && printf '# auth disabled\n' > /etc/nginx/auth-open.conf && nginx -t
EXPOSE 80
CMD ["/run.sh"]
