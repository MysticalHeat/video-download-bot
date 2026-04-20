package app

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"cobalt-telegram-bot/internal/cobalt"
	"cobalt-telegram-bot/internal/config"
	"cobalt-telegram-bot/internal/downloader"
	"cobalt-telegram-bot/internal/tiktok"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const maxPickerItems = 10

var urlPattern = regexp.MustCompile(`https?://\S+`)

type App struct {
	cfg        config.Config
	logger     *log.Logger
	bot        *tgbotapi.BotAPI
	cobalt     *cobalt.Client
	downloader *downloader.Downloader
	tiktok     *tiktok.Extractor
	jobs       chan struct{}
}

func New(cfg config.Config, logger *log.Logger) (*App, error) {
	bot, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		return nil, fmt.Errorf("create telegram bot: %w", err)
	}

	return &App{
		cfg:        cfg,
		logger:     logger,
		bot:        bot,
		cobalt:     cobalt.NewClient(cfg.CobaltAPIURL, cfg.CobaltAPIKey, cfg.RequestTimeout, cfg.VideoQuality),
		downloader: downloader.New(cfg.MaxUploadBytes, cfg.DownloadTimeout),
		tiktok:     tiktok.New(cfg.ProxyURL, cfg.MaxUploadBytes),
		jobs:       make(chan struct{}, cfg.MaxConcurrentJobs),
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	a.logger.Printf("bot authorized as @%s", a.bot.Self.UserName)

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 30

	updates := a.bot.GetUpdatesChan(updateConfig)
	defer a.bot.StopReceivingUpdates()

	var wg sync.WaitGroup

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return nil
		case update, ok := <-updates:
			if !ok {
				wg.Wait()
				return nil
			}

			message := update.Message
			if message == nil {
				continue
			}

			if message.IsCommand() {
				a.handleCommand(message)
				continue
			}

			if !a.shouldProcessMessage(message) {
				continue
			}

			text := strings.TrimSpace(messageText(message))
			if text == "" {
				continue
			}

			select {
			case a.jobs <- struct{}{}:
				wg.Add(1)
				go func(msg *tgbotapi.Message) {
					defer wg.Done()
					defer func() { <-a.jobs }()
					a.handleMessage(ctx, msg)
				}(message)
			default:
				a.replyText(message.Chat.ID, message.MessageID, "Сейчас занято. Попробуй ещё раз через минуту.")
			}
		}
	}
}

func (a *App) handleCommand(message *tgbotapi.Message) {
	switch message.Command() {
	case "start", "help":
		text := "Пришли ссылку на видео или пост. Я попробую скачать медиа через cobalt и отправить файл в ответ. Если файл окажется слишком большим для Telegram, пришлю прямую ссылку."
		a.replyText(message.Chat.ID, message.MessageID, text)
	default:
		a.replyText(message.Chat.ID, message.MessageID, "Не знаю такую команду. Пришли ссылку.")
	}
}

func (a *App) handleMessage(parent context.Context, message *tgbotapi.Message) {
	rawURL, ok := extractFirstURL(messageText(message))
	if !ok {
		a.replyText(message.Chat.ID, message.MessageID, "Нужна ссылка в сообщении.")
		return
	}

	ctx, cancel := context.WithTimeout(parent, a.cfg.DownloadTimeout+time.Minute)
	defer cancel()

	a.sendChatAction(message.Chat.ID, tgbotapi.ChatUploadDocument)
	a.replyText(message.Chat.ID, message.MessageID, "Обрабатываю ссылку…")

	if isTikTokURL(rawURL) {
		a.handleTikTok(ctx, message, rawURL)
		return
	}

	resolved, err := a.cobalt.Resolve(ctx, rawURL)
	if err != nil {
		if isCanceled(err) {
			return
		}
		a.logger.Printf("cobalt resolve error for %s: %v", rawURL, err)
		a.replyText(message.Chat.ID, message.MessageID, "Не получилось обратиться к cobalt. Попробуй позже.")
		return
	}

	if resolved.Status == "error" && resolved.Error != nil {
		a.replyText(message.Chat.ID, message.MessageID, humanizeCobaltError(resolved.Error.Code))
		return
	}

	switch resolved.Status {
	case "redirect", "tunnel":
		a.processSingleFile(ctx, message, resolved.URL, resolved.Filename, rawURL)
	case "picker":
		if len(resolved.Picker) == 0 {
			a.replyText(message.Chat.ID, message.MessageID, "Сервис не вернул медиа для этой ссылки.")
			return
		}
		count := len(resolved.Picker)
		if count > maxPickerItems {
			count = maxPickerItems
		}
		for i := 0; i < count; i++ {
			item := resolved.Picker[i]
			if item.URL == "" {
				continue
			}
			a.processSingleFile(ctx, message, item.URL, item.Filename, rawURL)
		}
	default:
		a.replyText(message.Chat.ID, message.MessageID, "Неожиданный ответ от cobalt. Попробуй другую ссылку.")
	}
}

func (a *App) handleTikTok(ctx context.Context, message *tgbotapi.Message, rawURL string) {
	file, tooLarge, err := a.tiktok.Download(ctx, rawURL)
	if err != nil {
		if isCanceled(err) {
			return
		}
		a.logger.Printf("tiktok extractor error for %s: %v", rawURL, err)
		freshURL, urlErr := a.tiktok.FreshURL(ctx, rawURL)
		if urlErr == nil && freshURL != "" {
			a.replyText(message.Chat.ID, message.MessageID, "Не получилось загрузить TikTok как файл. Вот прямая ссылка:\n"+freshURL)
			return
		}
		a.replyText(message.Chat.ID, message.MessageID, "Не получилось обработать TikTok ссылку.")
		return
	}
	if tooLarge {
		freshURL, err := a.tiktok.FreshURL(ctx, rawURL)
		if err != nil || freshURL == "" {
			a.replyText(message.Chat.ID, message.MessageID, "Файл слишком большой для Telegram, и не удалось получить прямую ссылку.")
			return
		}
		a.replyText(message.Chat.ID, message.MessageID, fmt.Sprintf("Файл слишком большой для загрузки в Telegram, вот прямая ссылка:\n%s", freshURL))
		return
	}
	defer os.RemoveAll(filepath.Dir(file.Path))

	a.sendChatAction(message.Chat.ID, chatActionForContentType(file.ContentType))
	if err := a.sendDownloadedFile(message.Chat.ID, message.MessageID, file, rawURL); err != nil {
		a.logger.Printf("telegram upload error for %s: %v", file.Path, err)
		freshURL, urlErr := a.tiktok.FreshURL(ctx, rawURL)
		if urlErr == nil && freshURL != "" {
			a.replyText(message.Chat.ID, message.MessageID, fmt.Sprintf("Не получилось загрузить файл в Telegram. Вот прямая ссылка:\n%s", freshURL))
			return
		}
		a.replyText(message.Chat.ID, message.MessageID, "Не получилось загрузить TikTok в Telegram.")
	}
}

func (a *App) processSingleFile(ctx context.Context, message *tgbotapi.Message, mediaURL string, filename string, sourceURL string) {
	file, tooLarge, err := a.downloader.Download(ctx, mediaURL, filename)
	if err != nil {
		if isCanceled(err) {
			return
		}
		freshURL, refreshedName, retried, retryErr := a.retryResolveAndDownload(ctx, sourceURL, mediaURL)
		if retryErr == nil {
			file, tooLarge, err = a.downloader.Download(ctx, freshURL, refreshedName)
			if err == nil {
				mediaURL = freshURL
				goto upload
			}
			if isCanceled(err) {
				return
			}
		}
		a.logger.Printf("download error for %s: %v", mediaURL, err)
		if retried {
			mediaURL = freshURL
		}
		a.replyText(message.Chat.ID, message.MessageID, "Не получилось скачать файл. Вот прямая ссылка:\n"+mediaURL)
		return
	}
	if tooLarge {
		freshURL := a.refetchMediaURL(ctx, sourceURL, mediaURL)
		a.replyText(message.Chat.ID, message.MessageID, fmt.Sprintf("Файл слишком большой для загрузки в Telegram, вот прямая ссылка:\n%s", freshURL))
		return
	}

upload:
	defer os.Remove(file.Path)

	a.sendChatAction(message.Chat.ID, chatActionForContentType(file.ContentType))

	if err := a.sendDownloadedFile(message.Chat.ID, message.MessageID, file, sourceURL); err != nil {
		a.logger.Printf("telegram upload error for %s: %v", file.Path, err)
		freshURL := a.refetchMediaURL(ctx, sourceURL, mediaURL)
		a.replyText(message.Chat.ID, message.MessageID, fmt.Sprintf("Не получилось загрузить файл в Telegram. Вот прямая ссылка:\n%s", freshURL))
	}
}

func (a *App) retryResolveAndDownload(ctx context.Context, sourceURL string, fallbackURL string) (string, string, bool, error) {
	resolved, err := a.cobalt.Resolve(ctx, sourceURL)
	if err != nil || resolved == nil {
		return fallbackURL, "", false, err
	}
	if resolved.Status != "redirect" && resolved.Status != "tunnel" {
		return fallbackURL, "", false, fmt.Errorf("unexpected cobalt status: %s", resolved.Status)
	}
	if resolved.URL == "" {
		return fallbackURL, "", false, fmt.Errorf("empty cobalt media url")
	}
	return resolved.URL, resolved.Filename, true, nil
}

func (a *App) refetchMediaURL(ctx context.Context, sourceURL string, fallbackURL string) string {
	resolved, err := a.cobalt.Resolve(ctx, sourceURL)
	if err != nil || resolved == nil {
		return fallbackURL
	}
	if resolved.Status == "redirect" || resolved.Status == "tunnel" {
		if resolved.URL != "" {
			return resolved.URL
		}
	}
	return fallbackURL
}

func (a *App) sendDownloadedFile(chatID int64, replyTo int, file *downloader.File, sourceURL string) error {
	reader, err := openTelegramFile(file)
	if err != nil {
		return err
	}
	defer reader.Reader.(*os.File).Close()

	caption := buildCaption(sourceURL)

	switch {
	case strings.HasPrefix(file.ContentType, "video/"):
		msg := tgbotapi.NewVideo(chatID, reader)
		msg.Caption = caption
		msg.SupportsStreaming = strings.HasSuffix(strings.ToLower(file.Name), ".mp4")
		msg.ReplyToMessageID = replyTo
		_, err = a.bot.Send(msg)
		return err
	case strings.HasPrefix(file.ContentType, "audio/"):
		msg := tgbotapi.NewAudio(chatID, reader)
		msg.Caption = caption
		msg.ReplyToMessageID = replyTo
		_, err = a.bot.Send(msg)
		return err
	case strings.HasPrefix(file.ContentType, "image/"):
		msg := tgbotapi.NewPhoto(chatID, reader)
		msg.Caption = caption
		msg.ReplyToMessageID = replyTo
		_, err = a.bot.Send(msg)
		return err
	default:
		msg := tgbotapi.NewDocument(chatID, reader)
		msg.Caption = caption
		msg.ReplyToMessageID = replyTo
		_, err = a.bot.Send(msg)
		return err
	}
}

func openTelegramFile(file *downloader.File) (tgbotapi.FileReader, error) {
	info, err := os.Stat(file.Path)
	if err != nil {
		return tgbotapi.FileReader{}, fmt.Errorf("stat downloaded file: %w", err)
	}
	if info.Size() <= 0 {
		return tgbotapi.FileReader{}, fmt.Errorf("downloaded file is empty")
	}

	fh, err := os.Open(file.Path)
	if err != nil {
		return tgbotapi.FileReader{}, fmt.Errorf("open downloaded file: %w", err)
	}

	return tgbotapi.FileReader{
		Name:   file.Name,
		Reader: fh,
	}, nil
}

func buildCaption(sourceURL string) string {
	caption := fmt.Sprintf("Источник: %s", sourceURL)
	if len([]rune(caption)) > 1024 {
		return "Источник: ссылка в исходном сообщении"
	}
	return caption
}

func (a *App) sendChatAction(chatID int64, action string) {
	_, err := a.bot.Request(tgbotapi.NewChatAction(chatID, action))
	if err != nil {
		a.logger.Printf("chat action error: %v", err)
	}
}

func (a *App) replyText(chatID int64, replyTo int, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyToMessageID = replyTo
	msg.DisableWebPagePreview = true
	if _, err := a.bot.Send(msg); err != nil {
		a.logger.Printf("send message error: %v", err)
	}
}

func extractFirstURL(text string) (string, bool) {
	match := urlPattern.FindString(text)
	if match == "" {
		return "", false
	}
	parsed, err := url.Parse(match)
	if err != nil || parsed.Host == "" {
		return "", false
	}
	return match, true
}

func messageText(message *tgbotapi.Message) string {
	text := strings.TrimSpace(message.Text)
	if text != "" {
		return text
	}
	return strings.TrimSpace(message.Caption)
}

func (a *App) shouldProcessMessage(message *tgbotapi.Message) bool {
	switch message.Chat.Type {
	case "private":
		return true
	case "group", "supergroup":
		return a.isReplyToBot(message) || a.hasBotMention(message)
	default:
		return false
	}
}

func (a *App) isReplyToBot(message *tgbotapi.Message) bool {
	return message.ReplyToMessage != nil && message.ReplyToMessage.From != nil && message.ReplyToMessage.From.ID == a.bot.Self.ID
}

func (a *App) hasBotMention(message *tgbotapi.Message) bool {
	username := strings.ToLower(strings.TrimSpace(a.bot.Self.UserName))
	text := strings.TrimSpace(message.Text)
	caption := strings.TrimSpace(message.Caption)

	if username != "" {
		needle := "@" + username
		if strings.Contains(strings.ToLower(text), needle) || strings.Contains(strings.ToLower(caption), needle) {
			return true
		}
	}

	if a.entityMentionsBot(text, message.Entities, username) || a.entityMentionsBot(caption, message.CaptionEntities, username) {
		return true
	}

	return false
}

func (a *App) entityMentionsBot(text string, entities []tgbotapi.MessageEntity, username string) bool {
	for _, entity := range entities {
		switch entity.Type {
		case "mention":
			mentioned := strings.ToLower(entityText(text, entity.Offset, entity.Length))
			if mentioned == "@"+username {
				return true
			}
		case "text_mention":
			if entity.User != nil && entity.User.ID == a.bot.Self.ID {
				return true
			}
		}
	}

	return false
}

func entityText(text string, offset, length int) string {
	runes := []rune(text)
	if offset < 0 || length <= 0 || offset >= len(runes) {
		return ""
	}

	end := offset + length
	if end > len(runes) {
		end = len(runes)
	}

	return string(runes[offset:end])
}

func isTikTokURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "tiktok.com" || host == "www.tiktok.com" || host == "vm.tiktok.com" || host == "vt.tiktok.com" || strings.HasSuffix(host, ".tiktok.com")
}

func humanizeCobaltError(code string) string {
	switch code {
	case "error.api.auth.key.missing", "error.api.auth.key.invalid":
		return "На сервере неправильно настроен API ключ cobalt."
	case "error.content.too.large":
		return "Файл слишком большой."
	case "error.link.unsupported":
		return "Эта ссылка не поддерживается cobalt."
	case "error.youtube.login.required":
		return "Для этой ссылки нужен логин/cookies на стороне cobalt."
	default:
		if code == "" {
			return "Не получилось обработать ссылку."
		}
		return "Не получилось обработать ссылку: " + code
	}
}

func chatActionForContentType(contentType string) string {
	switch {
	case strings.HasPrefix(contentType, "audio/"):
		return tgbotapi.ChatUploadVoice
	case strings.HasPrefix(contentType, "image/"), strings.HasPrefix(contentType, "video/"):
		return tgbotapi.ChatUploadVideo
	default:
		return tgbotapi.ChatUploadDocument
	}
}

func isCanceled(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}
