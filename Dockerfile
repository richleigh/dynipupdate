FROM python:3.11-slim

# Set working directory
WORKDIR /app

# Install system dependencies needed for netifaces
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    gcc \
    python3-dev \
    && rm -rf /var/lib/apt/lists/*

# Copy requirements and install Python dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy the application
COPY dynip_update.py .

# Make the script executable
RUN chmod +x dynip_update.py

# Run the script
CMD ["python", "dynip_update.py"]
