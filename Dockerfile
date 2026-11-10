# Один образ для обоих сервисов: внутри две программы — fetcher и bot.
# Какую запускать, указывается в docker-compose.yml (command).
#
# Multi-stage сборка: в первом этапе есть Go и исходники (~300 МБ),
# во второй, итоговый, попадают только готовые бинарники (~20 МБ).

# ---------- Этап 1: сборка ----------
FROM golang:1.25-alpine AS build

WORKDIR /src

# Сначала только go.mod/go.sum и скачивание зависимостей. Docker кэширует слои:
# пока зависимости не менялись, этот шаг при пересборке не повторяется.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=0 — статический бинарник, не зависит от системных библиотек.
# -trimpath убирает локальные пути из бинарника, -s -w — отладочную информацию (меньше размер).
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/fetcher ./cmd/fetcher && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/bot ./cmd/bot

# ---------- Этап 2: итоговый образ ----------
FROM alpine:3.20

# ca-certificates — чтобы работал HTTPS (Telegram, RSS), tzdata — часовые пояса.
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 10001 app

COPY --from=build /out/fetcher /out/bot /usr/local/bin/

# Запускаем не от root: если в программе найдётся уязвимость, у взломщика
# будет минимум прав внутри контейнера.
USER app
