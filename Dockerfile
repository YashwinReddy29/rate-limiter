FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o rate-limiter ./cmd/server

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/rate-limiter .
EXPOSE 50051 8080
CMD ["./rate-limiter"]