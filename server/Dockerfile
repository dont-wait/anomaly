FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /app/bin/server ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /app/bin/worker ./cmd/worker
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /app/bin/migrate ./cmd/migrate

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 appuser

WORKDIR /app
COPY --from=builder /app/go.mod ./go.mod
COPY --from=builder /app/bin/server ./server
COPY --from=builder /app/bin/worker ./worker
COPY --from=builder /app/bin/migrate ./migrate
COPY --from=builder /app/migrations ./migrations

USER appuser

EXPOSE 8080

CMD ["/app/server"]
