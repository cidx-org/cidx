# Multi-stage build for CIDX Docker image

# Stage 1: Build the binary
FROM golang:alpine AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o cidx ./cmd/cidx

# Stage 2: Runtime image
FROM docker:27-cli

# Install Docker CLI (already included in docker:cli)
# CIDX needs Docker to run tools in containers

# Copy CIDX binary from builder
COPY --from=builder /build/cidx /usr/local/bin/cidx

# Set working directory
WORKDIR /workspace

# Default command
ENTRYPOINT ["cidx"]
CMD ["--help"]

# Labels for GHCR
LABEL org.opencontainers.image.source="https://github.com/arcker/cidx"
LABEL org.opencontainers.image.description="CIDX - CI with Declarative eXecution"
LABEL org.opencontainers.image.licenses="MIT"
