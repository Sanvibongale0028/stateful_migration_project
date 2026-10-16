# ---------- Build stage ----------
FROM golang:1.27 AS builder

WORKDIR /app

# Copy dependency files first for better Docker layer caching
COPY go.mod go.sum ./

RUN go mod download

# Copy source code
COPY . .

# Build the Go rate limiter
RUN CGO_ENABLED=0 GOOS=linux go build -o rate-limiter ./cmd/server


# ---------- Runtime stage ----------
FROM alpine:3.22

WORKDIR /app

# CA certificates are useful for HTTP communication
RUN apk add --no-cache ca-certificates

# Copy compiled binary
COPY --from=builder /app/rate-limiter /app/rate-limiter

EXPOSE 4000

CMD ["/app/rate-limiter"]
