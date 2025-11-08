# Multi-stage build for minimal final image size
# Stage 1: Build the Go binary
FROM golang:1.21-alpine AS builder

# Install build dependencies and UPX
RUN apk add --no-cache \
    git \
    upx

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
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a \
    -installsuffix cgo \
    -o dynip-updater \
    main.go

# Compress the binary with UPX
# --best: maximum compression
# --lzma: use LZMA compression for better ratio
RUN upx --best --lzma dynip-updater

# Stage 2: Minimal runtime image using scratch
FROM scratch

# Copy the compressed binary from builder
COPY --from=builder /app/dynip-updater /dynip-updater

# Copy CA certificates for HTTPS requests
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Run the binary
ENTRYPOINT ["/dynip-updater"]
