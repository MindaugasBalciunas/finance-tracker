# nginx + the server in one container. The arm64 binaries and the built web
# app come from CI (.github/workflows/build.yaml → prebuilt/), where Go's
# compile cache is warm; compiling them here started from scratch every run.
FROM nginx:stable-alpine
WORKDIR /app
RUN apk add --no-cache ca-certificates jq openssl tzdata
COPY prebuilt/finance-tracker prebuilt/finance-tracker-mcp ./
COPY prebuilt/web/ /usr/share/nginx/html/
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
