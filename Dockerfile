# Build and runtime in one stage
FROM golang:1.25.1-alpine
RUN apk add --no-cache build-base ca-certificates
WORKDIR /app
# Copy source (vendor/ is excluded via .gcloudignore, data/ is included)
COPY . .
ENV CGO_ENABLED=1
RUN go build -v -o gca .
ENTRYPOINT ["./gca", "server", "--data", "/app/data"]
