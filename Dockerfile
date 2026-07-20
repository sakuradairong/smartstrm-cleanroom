FROM golang:1.26-alpine AS build
WORKDIR /src
ARG GOPROXY=https://proxy.golang.org,direct
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /smartstrm ./cmd/smartstrm

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata ffmpeg && adduser -D -u 10001 smartstrm
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
