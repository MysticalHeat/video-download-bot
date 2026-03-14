# cobalt Telegram bot

Go-бот для Telegram с long polling. Принимает ссылку, запрашивает media через self-hosted cobalt и пытается загрузить файл в Telegram. Если файл слишком большой для Bot API, отправляет прямую ссылку.

## Возможности

- long polling, без webhook
- загрузка media через cobalt API key
- TikTok обрабатывается отдельным extractor на `yt-dlp`
- fallback на прямую ссылку для больших файлов
- базовая поддержка `picker`-ответов cobalt
- подходит для запуска как `systemd` service

## Переменные окружения

Смотри `.env.example`.

Обязательные:

- `BOT_TOKEN`
- `COBALT_API_KEY`

## Локальный запуск

```bash
go mod tidy
go run ./cmd/cobalt-telegram-bot
```

## Сборка

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/cobalt-telegram-bot ./cmd/cobalt-telegram-bot
```

## systemd

Шаблон юнита: `deploy/systemd/cobalt-telegram-bot.service`
