# Multi-stage build to minimize final image size
FROM python:3.12-alpine AS builder

# Install build dependencies
RUN apk add --no-cache \
    gcc \
    musl-dev \
    linux-headers \
    python3-dev

# Set working directory
WORKDIR /app

# Copy requirements and install Python dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir --user -r requirements.txt

# Final stage - minimal runtime image
FROM python:3.12-alpine

# Set working directory
WORKDIR /app

# Copy Python packages from builder
COPY --from=builder /root/.local /root/.local

# Copy the application
COPY dynip_update.py .

# Make sure scripts in .local are usable
ENV PATH=/root/.local/bin:$PATH

# Run the script
CMD ["python", "dynip_update.py"]
