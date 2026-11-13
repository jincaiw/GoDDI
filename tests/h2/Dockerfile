# H2 acceptance image: the exact HEAD-built binary + web bundle (staged as
# .ha-goddi-bin / .ha-web-dist by `make h2-build`), no in-container frontend
# build. The node config is mounted at /etc/goddi/config.yaml by compose.
FROM alpine:3.24
RUN apk add --no-cache ca-certificates tzdata curl jq sqlite \
    && addgroup -S goddi && adduser -S -G goddi goddi
WORKDIR /app
COPY .ha-goddi-bin /usr/local/bin/goddi
COPY .ha-web-dist /app/web/dist
EXPOSE 6080 53/udp 53/tcp 9953
ENTRYPOINT ["goddi"]
CMD ["serve", "--config", "/etc/goddi/config.yaml"]
