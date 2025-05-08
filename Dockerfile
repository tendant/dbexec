# Stage 1: Build the binary
FROM golang:1.24 AS builder

WORKDIR /app

# Copy go.mod and go.sum first (to leverage Docker cache)
COPY go.mod ./
COPY go.sum ./
RUN go mod download

# Copy the rest of the source
COPY . ./

# Build the CLI binary
RUN CGO_ENABLED=0 GOOS=linux go build -o dbexec ./cmd/dbexec

# Stage 2: Create a minimal runtime image
FROM alpine:latest

# Add CA certificates in case it connects to remote DBs securely
RUN apk --no-cache add ca-certificates

# Copy binary from builder
COPY --from=builder /app/dbexec /app/dbexec

# Default command
ENTRYPOINT ["/app/dbexec"]