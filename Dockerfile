FROM golang:1.26.7-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/rate-limiter ./cmd/server

FROM alpine:3.23
RUN apk add --no-cache ca-certificates && addgroup -S app && adduser -S -G app app
COPY --from=builder /out/rate-limiter /usr/local/bin/rate-limiter
USER app
EXPOSE 8080 50051
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 CMD ["rate-limiter", "healthcheck"]
ENTRYPOINT ["rate-limiter"]
