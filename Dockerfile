# Multi-stage build for minimal final image size
# Stage 1: Build the Go binary
FROM golang:1.21-alpine AS builder

# Get the target architecture
ARG TARGETARCH

# Install build dependencies and build UPX from source
# This ensures consistent UPX availability across all platforms
RUN apk add --no-cache \
        git \
        build-base \
        cmake \
        ucl-dev \
        zlib-dev && \
    git clone --depth 1 --branch v4.2.4 https://github.com/upx/upx.git /tmp/upx && \
    cd /tmp/upx && \
    git submodule update --init --recursive --depth 1 && \
    make -j$(nproc) all && \
    cp /tmp/upx/build/release/upx /usr/local/bin/ && \
    chmod +x /usr/local/bin/upx && \
    cd / && \
    rm -rf /tmp/upx

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

# Compress the binary with UPX and clean up build dependencies
# --best: maximum compression
# --lzma: use LZMA compression for better ratio
RUN upx --best --lzma dynip-updater && \
    apk del git build-base cmake ucl-dev zlib-dev

# Stage 2: Minimal runtime image using scratch
FROM scratch

# Copy the compressed binary from builder
COPY --from=builder /app/dynip-updater /dynip-updater

# Copy CA certificates for HTTPS requests
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Run the binary
ENTRYPOINT ["/dynip-updater"]
