package downloader

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var unsafeNameChars = regexp.MustCompile(`[^a-zA-Z0-9._ -]+`)

type Downloader struct {
	httpClient *http.Client
	maxBytes   int64
}

const maxAttempts = 2

type File struct {
	Path        string
	Name        string
	Size        int64
	ContentType string
	SourceURL   string
}

func New(maxBytes int64, timeout time.Duration) *Downloader {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 10
	transport.MaxIdleConnsPerHost = 4

	return &Downloader{
		maxBytes: maxBytes,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}
}

func (d *Downloader) Download(ctx context.Context, rawURL string, suggestedName string) (*File, bool, error) {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		file, tooLarge, retry, err := d.downloadOnce(ctx, rawURL, suggestedName)
		if err == nil || tooLarge {
			return file, tooLarge, nil
		}
		if !retry {
			return nil, false, err
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, false, lastErr
	}

	return nil, false, fmt.Errorf("download failed")
}

func (d *Downloader) downloadOnce(ctx context.Context, rawURL string, suggestedName string) (*File, bool, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, false, fmt.Errorf("build download request: %w", err)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, false, true, fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, false, false, fmt.Errorf("download returned %s", resp.Status)
	}

	if resp.ContentLength > 0 && resp.ContentLength > d.maxBytes {
		return nil, true, false, nil
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	filename := chooseFilename(rawURL, suggestedName, contentType)

	tempFile, err := os.CreateTemp("", "cobalt-bot-*")
	if err != nil {
		return nil, false, false, fmt.Errorf("create temp file: %w", err)
	}

	cleanup := func() {
		tempFile.Close()
		_ = os.Remove(tempFile.Name())
	}

	var written int64
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			written += int64(n)
			if written > d.maxBytes {
				cleanup()
				return nil, true, false, nil
			}
			if _, err := tempFile.Write(buf[:n]); err != nil {
				cleanup()
				return nil, false, false, fmt.Errorf("write temp file: %w", err)
			}
		}

		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			cleanup()
			return nil, false, true, fmt.Errorf("read download body: %w", readErr)
		}
	}

	if written == 0 {
		cleanup()
		return nil, false, true, fmt.Errorf("downloaded file is empty")
	}

	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempFile.Name())
		return nil, false, false, fmt.Errorf("close temp file: %w", err)
	}

	finalPath := tempFile.Name() + filepath.Ext(filename)
	if err := os.Rename(tempFile.Name(), finalPath); err != nil {
		_ = os.Remove(tempFile.Name())
		return nil, false, false, fmt.Errorf("rename temp file: %w", err)
	}

	return &File{
		Path:        finalPath,
		Name:        filename,
		Size:        written,
		ContentType: contentType,
		SourceURL:   rawURL,
	}, false, false, nil
}

func chooseFilename(rawURL string, suggested string, contentType string) string {
	name := sanitizeFilename(strings.TrimSpace(suggested))
	if name == "" {
		if parsed, err := url.Parse(rawURL); err == nil {
			name = sanitizeFilename(path.Base(parsed.Path))
		}
	}
	if name == "" || name == "." || name == "/" {
		name = "download"
	}

	if filepath.Ext(name) == "" && contentType != "" {
		mediaType, _, _ := mime.ParseMediaType(contentType)
		if exts, _ := mime.ExtensionsByType(mediaType); len(exts) > 0 {
			name += exts[0]
		}
	}

	return name
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
