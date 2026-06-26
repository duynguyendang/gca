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

RUN apk add --no-cache ca-certificates wget

WORKDIR /app

# Copy binary, policies, prompts, and data from builder stage
COPY --from=builder /app/gca .
COPY --from=builder /app/prompts ./prompts
COPY --from=builder /app/policies ./policies
COPY --from=builder /app/data ./data

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -q --spider http://localhost:8080/api/health || exit 1

EXPOSE 8080

ENTRYPOINT ["./gca", "server", "--data", "/app/data"]
