package cobalt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	quality    string
}

type ResolveRequest struct {
	URL           string `json:"url"`
	VideoQuality  string `json:"videoQuality,omitempty"`
	FilenameStyle string `json:"filenameStyle,omitempty"`
	AlwaysProxy   bool   `json:"alwaysProxy"`
	DownloadMode  string `json:"downloadMode,omitempty"`
}

type ResolveResponse struct {
	Status   string       `json:"status"`
	URL      string       `json:"url"`
	Filename string       `json:"filename"`
	Picker   []PickerItem `json:"picker"`
	Error    *APIError    `json:"error"`
}

type PickerItem struct {
	Type     string `json:"type"`
	URL      string `json:"url"`
	Thumb    string `json:"thumb"`
	Filename string `json:"filename"`
}

type APIError struct {
	Code    string                 `json:"code"`
	Context map[string]interface{} `json:"context"`
}

func NewClient(baseURL string, apiKey string, timeout time.Duration, quality string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/") + "/",
		apiKey:  apiKey,
		quality: quality,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) Resolve(ctx context.Context, rawURL string) (*ResolveResponse, error) {
	body, err := json.Marshal(ResolveRequest{
		URL:           rawURL,
		VideoQuality:  c.quality,
		FilenameStyle: "pretty",
		AlwaysProxy:   true,
		DownloadMode:  "auto",
	})
	if err != nil {
		return nil, fmt.Errorf("marshal cobalt request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build cobalt request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Api-Key "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request cobalt: %w", err)
	}
	defer resp.Body.Close()

	var result ResolveResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode cobalt response: %w", err)
	}

	if result.Status == "error" && result.Error != nil {
		return &result, nil
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("cobalt returned %s", resp.Status)
	}

	return &result, nil
}
