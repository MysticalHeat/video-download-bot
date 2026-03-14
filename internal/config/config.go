package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultCobaltAPIURL    = "https://cobalt.nomli-com.ru/"
	defaultRequestTimeout  = 30 * time.Second
	defaultDownloadTimeout = 10 * time.Minute
	defaultMaxUploadBytes  = 48 * 1024 * 1024
	defaultMaxJobs         = 2
	defaultVideoQuality    = "720"
)

type Config struct {
	BotToken          string
	CobaltAPIURL      string
	CobaltAPIKey      string
	ProxyURL          string
	RequestTimeout    time.Duration
	DownloadTimeout   time.Duration
	MaxUploadBytes    int64
	MaxConcurrentJobs int
	VideoQuality      string
}

func Load() (Config, error) {
	cfg := Config{
		BotToken:          strings.TrimSpace(os.Getenv("BOT_TOKEN")),
		CobaltAPIURL:      withDefault(strings.TrimSpace(os.Getenv("COBALT_API_URL")), defaultCobaltAPIURL),
		CobaltAPIKey:      strings.TrimSpace(os.Getenv("COBALT_API_KEY")),
		ProxyURL:          strings.TrimSpace(os.Getenv("PROXY_URL")),
		RequestTimeout:    defaultRequestTimeout,
		DownloadTimeout:   defaultDownloadTimeout,
		MaxUploadBytes:    defaultMaxUploadBytes,
		MaxConcurrentJobs: defaultMaxJobs,
		VideoQuality:      withDefault(strings.TrimSpace(os.Getenv("COBALT_VIDEO_QUALITY")), defaultVideoQuality),
	}

	if cfg.BotToken == "" {
		return Config{}, fmt.Errorf("BOT_TOKEN is required")
	}
	if cfg.CobaltAPIKey == "" {
		return Config{}, fmt.Errorf("COBALT_API_KEY is required")
	}

	if value := strings.TrimSpace(os.Getenv("REQUEST_TIMEOUT")); value != "" {
		timeout, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("REQUEST_TIMEOUT: %w", err)
		}
		cfg.RequestTimeout = timeout
	}

	if value := strings.TrimSpace(os.Getenv("DOWNLOAD_TIMEOUT")); value != "" {
		timeout, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("DOWNLOAD_TIMEOUT: %w", err)
		}
		cfg.DownloadTimeout = timeout
	}

	if value := strings.TrimSpace(os.Getenv("MAX_UPLOAD_BYTES")); value != "" {
		size, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("MAX_UPLOAD_BYTES: %w", err)
		}
		cfg.MaxUploadBytes = size
	}

	if value := strings.TrimSpace(os.Getenv("MAX_CONCURRENT_JOBS")); value != "" {
		jobs, err := strconv.Atoi(value)
		if err != nil {
			return Config{}, fmt.Errorf("MAX_CONCURRENT_JOBS: %w", err)
		}
		if jobs < 1 {
			return Config{}, fmt.Errorf("MAX_CONCURRENT_JOBS must be >= 1")
		}
		cfg.MaxConcurrentJobs = jobs
	}

	return cfg, nil
}

func withDefault(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
