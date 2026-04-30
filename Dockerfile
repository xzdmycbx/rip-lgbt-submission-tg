# syntax=docker/dockerfile:1.7

# Frontend build
FROM node:20-alpine AS frontend-build
WORKDIR /app
COPY frontend/package.json frontend/package-lock.json* ./
RUN --mount=type=cache,target=/root/.npm \
    if [ -f package-lock.json ]; then npm ci; else npm install; fi
COPY frontend/ ./
RUN npm run build

# Backend build (pure-Go sqlite via modernc.org/sqlite)
FROM golang:1.22-alpine AS backend-build
RUN apk add --no-cache git
WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct
COPY backend/go.mod backend/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY backend/ ./
# Drop in the freshly built SPA bundle so it gets embedded.
COPY --from=frontend-build /app/dist ./internal/http/static
ENV CGO_ENABLED=0
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -ldflags "-s -w" -o /out/server ./cmd/server

# Runtime
FROM alpine:3.19
RUN apk add --no-cache chromium ca-certificates tzdata font-noto-cjk dumb-init \
 && adduser -D -u 1000 rip
ENV CHROMIUM_PATH=/usr/bin/chromium-browser
ENV TZ=Asia/Shanghai
ENV LISTEN_ADDR=":8080"
ENV DATA_DIR=/data
COPY --from=backend-build /out/server /usr/local/bin/server
WORKDIR /data
USER rip
EXPOSE 8080
ENTRYPOINT ["/usr/bin/dumb-init", "--"]
CMD ["/usr/local/bin/server"]
