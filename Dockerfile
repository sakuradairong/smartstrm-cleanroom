FROM node:26-bookworm AS web
WORKDIR /build
COPY web/package.json web/package-lock.json ./web/
RUN cd web && npm ci
COPY web ./web
RUN cd web && npm run build

FROM golang:1.23-alpine AS build
WORKDIR /src
ARG GOPROXY=https://proxy.golang.org,direct
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY --from=web /build/internal/app/ui/dist internal/app/ui/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /smartstrm ./cmd/smartstrm

FROM alpine:3.21
# Alpine's default CDN is slow from mainland China, where this image is built and
# run. Override APK_MIRROR to build against the upstream CDN or another mirror:
#   docker compose build --build-arg APK_MIRROR=dl-cdn.alpinelinux.org
ARG APK_MIRROR=mirrors.aliyun.com
RUN sed -i "s|dl-cdn.alpinelinux.org|${APK_MIRROR}|g" /etc/apk/repositories \
 && apk add --no-cache ca-certificates tzdata ffmpeg \
 && adduser -D -u 10001 smartstrm
COPY --from=build /smartstrm /usr/local/bin/smartstrm
COPY LICENSE /usr/share/licenses/smartstrm/LICENSE
LABEL org.opencontainers.image.title="SmartStrm Clean-room" \
      org.opencontainers.image.description="Independent clean-room STRM automation service" \
      org.opencontainers.image.source="https://github.com/sakuradairong/smartstrm-cleanroom" \
      org.opencontainers.image.licenses="AGPL-3.0-only"
USER smartstrm
EXPOSE 8024
ENTRYPOINT ["smartstrm"]
CMD ["-config", "/config/config.json"]
