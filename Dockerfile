ARG REGISTRY_MIRROR=docker.m.daocloud.io

FROM --platform=$BUILDPLATFORM ${REGISTRY_MIRROR}/library/node:24-alpine AS web-builder
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM ${REGISTRY_MIRROR}/library/golang:1.23-bookworm AS downloader-builder
ARG TARGETOS=linux
ARG TARGETARCH=amd64
WORKDIR /src
COPY assets/apple-music-downloader/go.mod assets/apple-music-downloader/go.sum ./
RUN go mod download
COPY assets/apple-music-downloader/ ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/apple-music-dl ./main.go

FROM ${REGISTRY_MIRROR}/library/ubuntu:24.04
ARG BENTO4_VERSION=1-6-0-641
ENV DEBIAN_FRONTEND=noninteractive \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    DATA_DIR=/data \
    DOWNLOAD_DIR=/downloads \
    WEB_DIR=/app/web \
    DEFAULT_CONFIG=/defaults/config.yaml.example \
    DOWNLOADER_BIN=/usr/local/bin/apple-music-dl \
    WRAPPER_MANAGER_URL=http://wrapper:18080 \
    PATH=/opt/venv/bin:$PATH

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl ffmpeg gpac python3 python3-venv unzip \
    && python3 -m venv /opt/venv \
    && curl -fsSL "https://www.bok.net/Bento4/binaries/Bento4-SDK-${BENTO4_VERSION}.x86_64-unknown-linux.zip" -o /tmp/bento4.zip \
    && unzip -q /tmp/bento4.zip -d /tmp/bento4 \
    && install -m 0755 /tmp/bento4/Bento4-SDK-${BENTO4_VERSION}.x86_64-unknown-linux/bin/mp4decrypt /usr/local/bin/mp4decrypt \
    && rm -rf /var/lib/apt/lists/* /tmp/bento4 /tmp/bento4.zip

WORKDIR /app/server
COPY server/requirements.txt /tmp/requirements.txt
RUN pip install --no-cache-dir -r /tmp/requirements.txt && rm /tmp/requirements.txt
COPY server/ /app/server/
COPY --from=web-builder /web/dist/ /app/web/
COPY --from=downloader-builder /out/apple-music-dl /usr/local/bin/apple-music-dl
COPY config.yaml.example /defaults/config.yaml.example
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN useradd --uid 10001 --create-home --shell /usr/sbin/nologin amdownload \
    && mkdir -p /data /downloads \
    && chown -R amdownload:amdownload /app /data /downloads /defaults \
    && chmod 0755 /usr/local/bin/docker-entrypoint.sh

VOLUME ["/data", "/downloads"]
EXPOSE 8080
HEALTHCHECK --interval=20s --timeout=3s --start-period=15s --retries=4 \
  CMD ["python", "-c", "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8080/healthz', timeout=2)"]
ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8080", "--no-access-log"]

LABEL org.opencontainers.image.title="AM download" \
      org.opencontainers.image.version="v1.0.4" \
      org.opencontainers.image.description="Apple Music download manager with a browser-based UI"
