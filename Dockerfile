ARG GO_VERSION=1.24.1
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine3.20 AS builder

RUN apk update && \
    apk add --no-cache git openssh-client upx

WORKDIR /build

COPY go.mod go.sum .
RUN go mod download

COPY . .

ARG VERSION="dev"
ARG BUILD_DATE
ARG COMMIT_SHA

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build \
    -trimpath \
    -ldflags="-w -s -X main.version=${VERSION} -X main.commit=${COMMIT_SHA} -X main.date=${BUILD_DATE}" \
    -tags=netgo,osusergo \
    -o /app/namira-core \
    ./cmd/namira-core/ \
    && upx --best --lzma /app/namira-core

FROM alpine:3.20

RUN apk update && \
    apk add --no-cache git openssh-client ca-certificates tzdata && \
    adduser -D -u 1000 namira && \
    mkdir -p /app/logs /home/namira/.ssh && \
    ssh-keyscan -H github.com >> /home/namira/.ssh/known_hosts && \
    chown -R namira:namira /app/logs /home/namira/.ssh && \
    chmod 700 /app/logs /home/namira/.ssh && \
    chmod 600 /home/namira/.ssh/known_hosts

WORKDIR /app
COPY --from=builder --chown=namira:namira /app/namira-core .

USER namira
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

LABEL org.opencontainers.image.title="namira-core" \
      org.opencontainers.image.description="Proxy configuration checker and validator" \
      org.opencontainers.image.url="https://github.com/NamiraNet/namira-core" \
      org.opencontainers.image.vendor="NamiraNet" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.revision="${COMMIT_SHA}"

ENTRYPOINT ["./namira-core"]
CMD ["api"]
