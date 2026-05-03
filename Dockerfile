# Build stage
FROM golang:1.26.2-alpine3.23 AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -o /driver \
    ./cmd/driver

# Runtime stage — distroless/static provides a minimal root environment
# without a shell, which is suitable for a statically linked Go binary.
FROM gcr.io/distroless/static-debian13

COPY --from=builder /driver /driver

ENTRYPOINT ["/driver"]
