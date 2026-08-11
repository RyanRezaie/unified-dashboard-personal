# ============================================================
# BUILD
# ============================================================
FROM golang:1.24-alpine AS build

WORKDIR /src
# No third-party dependencies, so there is no module download step to cache —
# go.mod is copied on its own only so a source-only change still skips it.
COPY go.mod ./
COPY . .

# Static binary: nothing in this program needs cgo, and a static build keeps
# the runtime image to a base plus one file.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/dashboard ./cmd/dashboard

# ============================================================
# RUNTIME
# ============================================================
FROM alpine:3.20

# tzdata is NOT optional: G.A.B. sends naive local timestamps and the server
# parses them with time.ParseInLocation(..., time.Local). Without zone data the
# container is UTC and every countdown is wrong by the offset. Set TZ below.
# ca-certificates is only needed if PROXMOX_INSECURE=false (a real cert).
RUN apk add --no-cache tzdata ca-certificates

ENV TZ=America/Chicago

# The frontend is embedded in the binary, so this is the whole deployment.
COPY --from=build /out/dashboard /usr/local/bin/dashboard

# Pipeline state is the only thing worth persisting. Mount a volume here.
ENV PIPELINE_PATH=/data/pipeline.json
VOLUME /data

# Not root: the process only needs to read its config and write /data.
RUN adduser -D -u 10001 dashboard && mkdir -p /data && chown dashboard /data
USER dashboard

EXPOSE 8088
HEALTHCHECK --interval=30s --timeout=3s \
  CMD wget -qO- http://127.0.0.1:8088/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/dashboard"]
