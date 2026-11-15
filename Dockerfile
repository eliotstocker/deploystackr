# syntax=docker/dockerfile:1

# Stage 1: Build Frontend (Svelte 5 SPA)
FROM --platform=$BUILDPLATFORM node:20-alpine AS frontend-builder
WORKDIR /app/web

COPY web/package*.json ./
RUN npm ci

COPY web/ ./
RUN npm run build

# Stage 2: Compile Backend (Go 1.22+)
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS backend-builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Copy compiled frontend assets over web/dist for go:embed
COPY --from=frontend-builder /app/web/dist ./web/dist

RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/deploystackr ./cmd/server

# Stage 3: Runtime
FROM alpine:3.20

# Install git, openssh-client, gnupg, ca-certificates for headless Git operations and commit signing
RUN apk add --no-cache git openssh-client gnupg ca-certificates tzdata

# Create non-root user and directories
RUN addgroup -g 10001 -S appuser && \
    adduser -u 10001 -S appuser -G appuser && \
    mkdir -p /data/scratch /home/appuser/.ssh /etc/deploystackr/ssh && \
    chown -R appuser:appuser /data /home/appuser && \
    chmod 700 /home/appuser/.ssh && \
    git config --system --add safe.directory '*'

COPY --from=backend-builder /out/deploystackr /usr/local/bin/deploystackr

COPY docker/git-askpass.sh /usr/local/bin/git-askpass.sh
RUN chmod +x /usr/local/bin/git-askpass.sh

USER appuser
WORKDIR /home/appuser

ENV PORT=8080 \
    SCRATCH_DIR=/data/scratch \
    CONFIG_PATH=/etc/deploystackr/config.yaml

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/deploystackr"]
