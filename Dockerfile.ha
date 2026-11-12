# Minimal HA-exercise image: reuses the exact binary and web bundle that the
# local verification matrix ran against, instead of rebuilding the frontend
# inside the container (the in-container pnpm install cannot reach the
# registry from this sandbox -- see the stuck multi-stage attempt, round 246).
# The multi-stage Dockerfile remains the production build path (CI builds it).
FROM alpine:3.24

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S goddi \
    && adduser -S -G goddi goddi

WORKDIR /app

# The binary is staged into the context as .ha-goddi-bin because .dockerignore
# excludes dist/ (and the bare name goddi). scripts/e2e_local.sh or the round
# 246 runbook step does: cp dist/goddi .ha-goddi-bin
COPY .ha-goddi-bin /usr/local/bin/goddi
COPY .ha-web-dist /app/web/dist
COPY config.yaml /etc/goddi/config.yaml
USER goddi

EXPOSE 6080 53/udp 53/tcp

ENTRYPOINT ["goddi"]
CMD ["serve", "--config", "/etc/goddi/config.yaml"]
