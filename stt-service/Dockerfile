FROM golang:1.24-bullseye AS builder

RUN apt-get update && apt-get install -y gcc musl-dev && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build -o /app/bin/main ./stt-server/main.go

FROM debian:bullseye-slim

RUN apt-get update && apt-get install -y \
    ffmpeg \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /var/lib/stt /opt/stt/migrations /opt/stt/web/dist /opt/stt/models

WORKDIR /opt/stt

COPY --from=builder /app/bin/main ./main

COPY --from=builder /app/migrations ./migrations
COPY --from=builder /app/web/dist ./web/dist

COPY --from=builder /app/whisper-cli /opt/stt/whisper-cli

EXPOSE 8080

CMD ["./main"]
