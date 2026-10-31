# Anasaki Studio CMS — what every site owner runs on their own server.
#   docker build -t anasaki-studio .              (version from the VERSION file)
#   docker build --build-arg VERSION=2.0.0-rc.1 -t anasaki-studio:2.0.0-rc.1 .   (override)
# Multi-arch friendly: the web and Go stages run on the build machine's
# architecture and Go cross-compiles for the target (no slow emulation).

# --- Angular admin + designer (architecture independent output)
FROM --platform=$BUILDPLATFORM node:24-alpine AS web
WORKDIR /web
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npx ng build

# --- Go CMS server
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS api
ARG TARGETOS TARGETARCH VERSION=
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY VERSION /VERSION
RUN v="${VERSION:-$(tr -d ' \r\n' < /VERSION)}" && echo "version $v" && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags="-s -w -X github.com/anasaki/studio/backend/internal/config.Version=${v}" \
    -o /out/server ./cmd/server

FROM alpine:3.22
ARG VERSION=
LABEL org.opencontainers.image.title="Anasaki Studio" \
      org.opencontainers.image.description="Website builder and CMS with a visual designer and plugins" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}"
RUN apk add --no-cache ca-certificates tzdata wget && adduser -D app && mkdir -p /data/uploads /data/plugins && chown -R app /data
COPY --from=api /out/server /app/server
COPY --from=web /web/dist/anasaki-studio/browser /app/admin
USER app
WORKDIR /app
ENV ADDR=:8090 ADMIN_DIR=/app/admin UPLOAD_DIR=/data/uploads PLUGINS_DIR=/data/plugins
EXPOSE 8090
VOLUME ["/data/uploads", "/data/plugins"]
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s CMD wget -qO- http://127.0.0.1:8090/api/health >/dev/null || exit 1
ENTRYPOINT ["/app/server"]
