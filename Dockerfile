# Multi-stage build for minimal final image size
# Stage 1: Build the Go binary
FROM golang:1.21-alpine AS builder

# Get the target architecture
ARG TARGETARCH

# Try to install UPX from apk if available for this architecture
# This will succeed on supported architectures and fail silently on unsupported ones
RUN apk add --no-cache upx || true

# Set working directory
WORKDIR /app

# Copy Go module files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY main.go .

# Build the binary with optimizations for size
# - Disable CGO for static binary
# - Strip debug info and symbol table
# - Disable DWARF generation
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a \
    -installsuffix cgo \
    -o dynip-updater \
    main.go

# Compress the binary with UPX if available
# UPX may not be available on all architectures (e.g., riscv64, s390x)
# --best: maximum compression
# --lzma: use LZMA compression for better ratio
RUN if command -v upx >/dev/null 2>&1; then \
        upx --best --lzma dynip-updater; \
    fi

# Stage 2: Minimal runtime image using scratch
FROM scratch

# Copy the compressed binary from builder
COPY --from=builder /app/dynip-updater /dynip-updater

# Copy CA certificates for HTTPS requests
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Run the binary
ENTRYPOINT ["/dynip-updater"]
