FROM golang:1.24.1-alpine3.21 AS builder

WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum* ./

# Download dependencies if go.sum exists
RUN if [ -f go.sum ]; then go mod download; fi

# Copy source code
COPY *.go ./

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o blockchain-websocket-exporter .

# Use a minimal alpine image for the final stage
FROM alpine:3.21

WORKDIR /

# Copy binary from builder stage
COPY --from=builder /app/blockchain-websocket-exporter /blockchain-websocket-exporter

# Expose the default port
EXPOSE 9095

# Run the exporter
ENTRYPOINT ["/blockchain-websocket-exporter"]