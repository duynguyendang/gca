# Stage 1: Build the Go binary
FROM golang:1.25.1-alpine AS builder
# Install build tools for CGO (required by tree-sitter)
RUN apk add --no-cache build-base
WORKDIR /app
COPY . .
# Enable CGO for tree-sitter bindings
ENV CGO_ENABLED=1
RUN go build -o gca .

# Stage 2: Minimal Runtime (No build tools needed)
FROM alpine:latest
RUN apk add --no-cache ca-certificates
WORKDIR /root/
COPY --from=builder /app/gca /usr/local/bin/
# Include your pre-ingested BadgerDB and vector data
COPY --from=builder /app/data /data 
# Copy prompts for AI service (relative to WORKDIR /root/)
COPY --from=builder /app/prompts ./prompts


# Start in server mode
ENTRYPOINT ["gca", "--server", "/data"] 
