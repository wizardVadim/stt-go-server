FROM golang:1.27-bullseye AS builder

RUN apt-get update && apt-get install -y gcc musl-dev && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY stt-service/go.mod stt-service/go.sum ./
RUN go mod download

COPY stt-service/ ./stt-service/
COPY migrations/ ./migrations/

RUN CGO_ENABLED=1 GOOS=linux go build -o /app/bin/main ./stt-service/cmd/main.go

FROM debian:bullseye-slim

RUN apt-get update && apt-get install -y \
    ffmpeg \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /var/lib/stt /opt/stt/migrations /opt/stt/web/dist /opt/stt/models

WORKDIR /opt/stt

COPY --from=builder /app/bin/main ./main
COPY --from=builder /app/migrations ./migrations

# ВНИМАНИЕ: Если whisper-cli — это внешний бинарник, который вы скачиваете вручную,
# его нужно положить в проект и раскомментировать строку ниже. 
# Пока закомментировано, чтобы не вызывать ошибку "no such file or directory" при сборке.
# COPY --from=builder /app/whisper-cli /opt/stt/whisper-cli

EXPOSE 8080

CMD ["./main"]
