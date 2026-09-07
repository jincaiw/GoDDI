# Frontend build stage (soybean-admin pnpm monorepo)
FROM node:22-alpine AS web-builder

RUN corepack enable && corepack prepare pnpm@10.15.0 --activate

WORKDIR /app/web-admin

# Copy source code
COPY web-admin/ ./
RUN pnpm install --frozen-lockfile
RUN pnpm build

# Backend build stage
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .
COPY --from=web-builder /app/web-admin/dist/ /app/web/dist/

# Build binary with version info
ARG VERSION=0.1.2
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags \
    "-X main.Version=${VERSION} \
     -X main.GitCommit=${GIT_COMMIT} \
     -X main.BuildDate=${BUILD_DATE} \
     -s -w" \
    -o /goddi ./cmd/goddi

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S goddi \
    && adduser -S -G goddi goddi

WORKDIR /app

# The binary embeds the web UI and database migrations.
COPY --from=builder /goddi /usr/local/bin/goddi

# The image runs as the non-root "goddi" user but must bind the privileged
# DNS (:53) and DHCP (:67) ports. A file capability survives exec for
# non-root users (unlike cap_add, which is dropped on exec for non-root).
RUN apk add --no-cache libcap \
    && setcap cap_net_bind_service=+ep /usr/local/bin/goddi

# Create data directory
RUN mkdir -p /var/lib/goddi && chown -R goddi:goddi /var/lib/goddi

# Copy default config
COPY config.yaml /etc/goddi/config.yaml

USER goddi

EXPOSE 6080 53/udp 53/tcp 853/tcp 8443/tcp

VOLUME ["/var/lib/goddi", "/etc/goddi"]

ENTRYPOINT ["goddi"]
CMD ["serve", "--config", "/etc/goddi/config.yaml"]
