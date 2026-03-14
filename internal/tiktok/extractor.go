package tiktok

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"cobalt-telegram-bot/internal/downloader"
)

var unsafeNameChars = regexp.MustCompile(`[^a-zA-Z0-9._ -]+`)

type Extractor struct {
	proxyURL string
	maxBytes int64
}

type metadata struct {
	ID       string `json:"id"`
	Uploader string `json:"uploader"`
	Ext      string `json:"ext"`
	Title    string `json:"title"`
	Filename string `json:"_filename"`
	URL      string `json:"url"`
	Protocol string `json:"protocol"`
	Formats  []struct {
		URL    string `json:"url"`
		Ext    string `json:"ext"`
		Format string `json:"format_id"`
	} `json:"formats"`
}

func New(proxyURL string, maxBytes int64) *Extractor {
	return &Extractor{proxyURL: proxyURL, maxBytes: maxBytes}
}

func (e *Extractor) Download(ctx context.Context, rawURL string) (*downloader.File, bool, error) {
	dir, err := os.MkdirTemp("", "tiktok-bot-*")
	if err != nil {
		return nil, false, fmt.Errorf("create temp dir: %w", err)
	}

	outputTemplate := filepath.Join(dir, "%(uploader)s_%(id)s.%(ext)s")
	args := []string{
		"--no-playlist",
		"--no-progress",
		"--newline",
		"--retries", "3",
		"--fragment-retries", "5",
		"--buffer-size", "16K",
		"--user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36",
		"-f", "best[ext=mp4]/best",
		"-o", outputTemplate,
		rawURL,
	}
	if e.proxyURL != "" {
		args = append([]string{"--proxy", e.proxyURL}, args...)
	}

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.RemoveAll(dir)
		return nil, false, fmt.Errorf("yt-dlp download failed: %v: %s", err, strings.TrimSpace(stderr.String()))
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, false, fmt.Errorf("read temp dir: %w", err)
	}
	if len(entries) == 0 {
		_ = os.RemoveAll(dir)
		return nil, false, fmt.Errorf("yt-dlp produced no files")
	}

	var mediaPath string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if strings.HasSuffix(name, ".part") || strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".description") {
			continue
		}
		mediaPath = filepath.Join(dir, entry.Name())
		break
	}
	if mediaPath == "" {
		_ = os.RemoveAll(dir)
		return nil, false, fmt.Errorf("yt-dlp media file not found")
	}

	info, err := os.Stat(mediaPath)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, false, fmt.Errorf("stat media file: %w", err)
	}
	if info.Size() > e.maxBytes {
		_ = os.RemoveAll(dir)
		return nil, true, nil
	}

	name := sanitizeFilename(filepath.Base(mediaPath))
	if name == "" {
		name = "tiktok.mp4"
	}

	return &downloader.File{
		Path:        mediaPath,
		Name:        name,
		Size:        info.Size(),
		ContentType: "video/mp4",
		SourceURL:   rawURL,
	}, false, nil
}

func (e *Extractor) FreshURL(ctx context.Context, rawURL string) (string, error) {
	args := []string{"--dump-single-json", "--no-download", "--user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36", rawURL}
	if e.proxyURL != "" {
		args = append([]string{"--proxy", e.proxyURL}, args...)
	}

	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("yt-dlp metadata failed: %v: %s", err, strings.TrimSpace(stderr.String()))
	}

	var meta metadata
	if err := json.Unmarshal(stdout.Bytes(), &meta); err != nil {
		return "", fmt.Errorf("decode yt-dlp metadata: %w", err)
	}
	if meta.URL != "" {
		return meta.URL, nil
	}
	for _, format := range meta.Formats {
		if format.Ext == "mp4" && format.URL != "" {
			return format.URL, nil
		}
	}
	return "", fmt.Errorf("no downloadable URL in yt-dlp metadata")
}

func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, string(filepath.Separator), "_")
	name = unsafeNameChars.ReplaceAllString(name, "_")
	name = strings.Trim(name, " ._")
	if len(name) > 180 {
		name = name[:180]
	}
	return name
}
