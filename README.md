# video-download-bot

Лёгкий Telegram-бот на Go для скачивания медиа по ссылке.

- **Instagram / YouTube / другие поддерживаемые сервисы** → через self-hosted `cobalt`
- **TikTok** → через `yt-dlp`
- если файл слишком большой для Telegram → бот отправляет **прямую ссылку**

## Что умеет

- long polling, без webhook
- загрузка видео/аудио/изображений в ответ на ссылку
- fallback на прямую ссылку при превышении лимита Telegram
- отдельный TikTok extractor
- поддержка групповых чатов
- запуск как `systemd` service

## Как это работает

```text
Telegram message
  ├─ TikTok link      -> yt-dlp
  └─ Other supported  -> cobalt API
                       -> download file
                       -> upload to Telegram
                       -> fallback to direct URL if needed
```

## Требования

- Go 1.22+
- self-hosted `cobalt`
- `yt-dlp`
- `ffmpeg`
- Telegram bot token

## Переменные окружения

Смотри `.env.example`.

### Обязательные

- `BOT_TOKEN`
- `COBALT_API_KEY`

### Основные

| Variable | Description |
|---|---|
| `BOT_TOKEN` | токен Telegram-бота |
| `COBALT_API_URL` | URL self-hosted cobalt |
| `COBALT_API_KEY` | API key для cobalt |
| `PROXY_URL` | proxy для TikTok extractor / yt-dlp |
| `REQUEST_TIMEOUT` | timeout запросов к cobalt |
| `DOWNLOAD_TIMEOUT` | timeout на скачивание файла |
| `MAX_UPLOAD_BYTES` | максимальный размер файла для загрузки в Telegram |
| `MAX_CONCURRENT_JOBS` | число одновременных задач |
| `COBALT_VIDEO_QUALITY` | желаемое качество видео для cobalt |

## Локальный запуск

```bash
go mod tidy
go run ./cmd/cobalt-telegram-bot
```

## Сборка

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o dist/cobalt-telegram-bot ./cmd/cobalt-telegram-bot
```

## Прод-запуск через systemd

Шаблон юнита:

- `deploy/systemd/cobalt-telegram-bot.service`

Типичный layout на сервере:

```text
/opt/cobalt-telegram-bot/cobalt-telegram-bot
/etc/cobalt-telegram-bot.env
/etc/systemd/system/cobalt-telegram-bot.service
```

После обновления бинаря:

```bash
systemctl restart cobalt-telegram-bot
journalctl -u cobalt-telegram-bot -f
```

## Работа в группах

Бот умеет работать в группах, но для нормального поведения лучше:

1. отключить **privacy mode** через `@BotFather`
2. удалить бота из группы
3. добавить заново

Сейчас бот реагирует на:

- `@mention`
- reply к сообщению бота
- обычные ссылки в группе после отключения privacy mode

## Структура проекта

```text
cmd/cobalt-telegram-bot/       # entrypoint
internal/app/                  # bot runtime and handlers
internal/cobalt/               # cobalt API client
internal/tiktok/               # TikTok extractor via yt-dlp
internal/downloader/           # media downloader
internal/config/               # env config
deploy/systemd/                # service template
```

## Примечания

- токены и ключи не хранятся в репозитории
- для TikTok надёжнее использовать отдельный extractor, чем пытаться вести всё через cobalt
- если Telegram не принимает файл, бот старается отдать пользователю свежую прямую ссылку
