FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache gcc musl-dev

WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the dsn binary
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dsn ./cmd/dsn

# Final stage
FROM alpine:3.19

# Install CA certificates and runtime dependencies
RUN apk add --no-cache ca-certificates dumb-init

WORKDIR /root

# Copy binary from builder
COPY --from=builder /build/dsn /usr/local/bin/dsn

# Create data directory
RUN mkdir -p /root/.dsn/data

# Create log directory
RUN mkdir -p /root/.dsn/logs

# Set environment variables
ENV PATH="/usr/local/bin:${PATH}"
ENV HOME="/root"

# Expose ports
EXPOSE 26656 8545 9464

# Use dumb-init to handle signals properly
ENTRYPOINT ["dumb-init", "--"]

# Run the dsn node
CMD ["node", "start"]