// Package paste publishes a saved results file to a paste service so the run
// can be shared through a single short link.
//
// The default service is https://paste.rs: it accepts the raw bytes of the file
// as the body of a POST request and answers with the URL of the created paste.
package paste

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// DefaultEndpoint is the paste.rs upload endpoint. POSTing a file's raw
// contents to it returns the URL of the new paste in the response body.
const DefaultEndpoint = "https://paste.rs/"

// maxResponseBytes bounds how much of the paste service response is read; the
// whole reply is expected to be the link itself.
const maxResponseBytes = 4096

// Prompt is the question asked once the results file has been saved. The
// default answer is "no": only an explicit yes publishes the file.
const Prompt = "Upload the saved results to paste.rs and get a shareable link? [y/N]: "

// IsAffirmative reports whether an answer to Prompt means yes. The check is
// case-insensitive and whitespace-tolerant, so y, Y, yes, YES and Yes all
// count, while everything else (including an empty answer or EOF) means no.
func IsAffirmative(answer string) bool {
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// Client uploads files to a paste service.
type Client struct {
	Endpoint string
	HTTP     *http.Client
}

// NewClient builds a Client for endpoint. An empty endpoint falls back to
// DefaultEndpoint.
func NewClient(endpoint string) *Client {
	if strings.TrimSpace(endpoint) == "" {
		endpoint = DefaultEndpoint
	}
	return &Client{
		Endpoint: endpoint,
		HTTP:     &http.Client{Timeout: 60 * time.Second},
	}
}

// UploadFile reads path and uploads its contents, returning the paste link.
//
// A missing or empty file is reported as ("", nil): the results file is only
// created once a healthy configuration is found, so a run that produced no
// results must not be treated as an upload error. The caller is expected to
// treat an empty link as "nothing to publish".
func (c *Client) UploadFile(ctx context.Context, path string) (string, error) {
	if c == nil {
		c = NewClient("")
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	endpoint := c.Endpoint
	if strings.TrimSpace(endpoint) == "" {
		endpoint = DefaultEndpoint
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read results file %s: %w", path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return "", nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to build paste request: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	req.Header.Set("User-Agent", "v2pingo")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to upload results to %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("failed to read paste response from %s: %w", endpoint, err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("paste service %s returned status %d", endpoint, resp.StatusCode)
	}

	link := strings.TrimSpace(string(body))
	if !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
		return "", fmt.Errorf("paste service %s returned an unexpected response: %q", endpoint, link)
	}
	return link, nil
}
