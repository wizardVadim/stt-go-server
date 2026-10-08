# --- Сборка Go-приложения ---
FROM golang:1.27-bookworm AS builder

WORKDIR /app

# Зависимости
COPY stt-service/go.mod stt-service/go.sum ./
RUN go mod download

# Исходники Go
COPY stt-service/ ./

# Миграции
COPY migrations/ ./migrations/

# Компиляция
RUN CGO_ENABLED=1 GOOS=linux go build -o /app/bin/main ./cmd

# --- Финальный образ ---
FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ffmpeg \
        ca-certificates && \
    rm -rf /var/lib/apt/lists/*

RUN mkdir -p \
    /var/lib/stt \
    /opt/stt/migrations \
    /opt/stt/web/dist \
    /opt/stt/models

WORKDIR /opt/stt

COPY --from=builder /app/bin/main ./main
COPY --from=builder /app/migrations ./migrations

EXPOSE 8080

CMD ["./main"]