# Stage 1: Build
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache build-base ca-certificates

WORKDIR /app

# Copy only go mod files first for dependency caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
ENV CGO_ENABLED=1
RUN go build -v -o gca .

# Stage 2: Runtime
FROM alpine:3.19

RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/gca .
COPY --from=builder /app/data ./data

ENTRYPOINT ["./gca", "server", "--data", "/app/data"]