# --- Этап сборки приложения ---
FROM golang:1.27-bookworm AS builder

WORKDIR /app

# Копируем зависимости
COPY stt-service/go.mod stt-service/go.sum ./
RUN go mod download

# Копируем исходники и миграции
COPY stt-service/ ./stt-service/
COPY migrations/ ./migrations/

# Собираем бинарник
RUN CGO_ENABLED=1 GOOS=linux go build -o /app/bin/main ./stt-service/cmd/main.go

# --- Финальный легкий образ ---
FROM debian:bookworm-slim

# Ставим только самое нужное (ffmpeg ставится одной строкой и без сбоев)
RUN apt-get update && apt-get install -y ffmpeg ca-certificates && rm -rf /var/lib/apt/lists/*

# Создаем структуру папок
RUN mkdir -p /var/lib/stt /opt/stt/migrations /opt/stt/web/dist /opt/stt/models
WORKDIR /opt/stt

# Переносим результаты сборки
COPY --from=builder /app/bin/main ./main
COPY --from=builder /app/migrations ./migrations

EXPOSE 8080

CMD ["./main"]
