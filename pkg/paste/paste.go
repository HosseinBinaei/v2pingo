// Package paste publishes a saved results file to a paste service so the run
// can be shared through a single short link.
//
// The default service is https://paste.rs: it accepts the raw bytes of the file
// as the body of a POST request and answers with the URL of the created paste.
// Because the service is out of the project's control, every upload also has
// fallback services: if paste.rs fails (HTTP error, timeout, DNS/TLS problem or
// any other connectivity issue), the same file is tried against the next
// service until one of them returns a link.
package paste

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// DefaultEndpoint is the paste.rs upload endpoint. POSTing a file's raw
// contents to it returns the URL of the new paste in the response body.
const DefaultEndpoint = "https://paste.rs/"

// cnetEndpoint accepts the file's raw bytes exactly like paste.rs.
const cnetEndpoint = "https://paste.c-net.org/"

// dpasteEndpoint speaks the dpaste.org form API instead of a raw body.
const dpasteEndpoint = "https://dpaste.org/api/"

// maxResponseBytes bounds how much of the paste service response is read; the
// whole reply is expected to be the link itself.
const maxResponseBytes = 4096

// Prompt is the question asked once the results file has been saved. The
// default answer is "no": only an explicit yes publishes the file.
const Prompt = "Upload the saved results to paste.rs and get a shareable link? [y/N]: "

// RetryPrompt is the question asked when an upload attempt failed. Like Prompt
// it defaults to "no", so a failed run never loops unless the user asks for
// another attempt explicitly.
const RetryPrompt = "Upload failed. Retry uploading to paste.rs? [y/N]: "

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

// service is one paste provider: where to POST and how to turn the file bytes
// into that provider's upload request.
type service struct {
	endpoint string
	build    func(data []byte) (*http.Request, error)
}

// rawBodyRequest builds the paste.rs-style request used by paste.rs and
// paste.c-net.org: the file bytes are the request body itself.
func rawBodyRequest(data []byte) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodPost, "", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	return req, nil
}

// dpasteRequest builds the dpaste.org API request: the file travels in the
// form-encoded content field, and format=url makes the API answer with the
// paste link in the response body.
func dpasteRequest(data []byte) (*http.Request, error) {
	form := url.Values{}
	form.Set("content", string(data))
	form.Set("lexer", "text")
	form.Set("format", "url")
	req, err := http.NewRequest(http.MethodPost, "", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req, nil
}

// fallbackServices are tried, in order, after the primary service fails. Both
// were verified to accept a plain results file and answer with the paste URL.
var fallbackServices = []service{
	{endpoint: cnetEndpoint, build: rawBodyRequest},
	{endpoint: dpasteEndpoint, build: dpasteRequest},
}

// Client uploads files to a paste service, trying fallbacks in order when the
// primary endpoint fails.
type Client struct {
	Endpoint string
	HTTP     *http.Client

	// fallbacks holds the services tried after the primary one fails. It is
	// set by NewClient for the default endpoint, empty for an explicitly
	// configured endpoint, and replaced by tests to inject mock services.
	fallbacks []service
}

// NewClient builds a Client for endpoint. An empty endpoint falls back to
// DefaultEndpoint and gets the built-in fallback services; an explicitly
// configured endpoint is used alone, exactly as configured.
func NewClient(endpoint string) *Client {
	trimmed := strings.TrimSpace(endpoint)
	explicit := trimmed != "" && endpoint != DefaultEndpoint
	if trimmed == "" {
		endpoint = DefaultEndpoint
	}
	c := &Client{
		Endpoint: endpoint,
		HTTP:     &http.Client{Timeout: 60 * time.Second},
	}
	if !explicit {
		c.fallbacks = fallbackServices
	}
	return c
}

// serviceList returns the services to try, in order: the configured primary
// endpoint first, then the fallbacks. A fallback whose endpoint duplicates the
// primary is skipped so the same service is never asked twice.
func (c *Client) serviceList() []service {
	endpoint := c.Endpoint
	if strings.TrimSpace(endpoint) == "" {
		endpoint = DefaultEndpoint
	}
	primary := service{endpoint: endpoint, build: rawBodyRequest}
	list := make([]service, 0, len(c.fallbacks)+1)
	for _, s := range c.fallbacks {
		if s.endpoint == primary.endpoint {
			primary = s
			break
		}
	}
	list = append(list, primary)
	for _, s := range c.fallbacks {
		if s.endpoint != primary.endpoint {
			list = append(list, s)
		}
	}
	return list
}

// UploadFile reads path and uploads its contents, returning the link of the
// first paste service that accepts the file.
//
// A missing or empty file is reported as ("", nil): the results file is only
// created once a healthy configuration is found, so a run that produced no
// results must not be treated as an upload error. The caller is expected to
// treat an empty link as "nothing to publish".
//
// When every service fails the error names each endpoint that was tried.
func (c *Client) UploadFile(ctx context.Context, path string) (string, error) {
	if c == nil {
		c = NewClient("")
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
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

	var failures []string
	for _, s := range c.serviceList() {
		link, err := uploadTo(ctx, client, s, data)
		if err == nil {
			return link, nil
		}
		failures = append(failures, fmt.Sprintf("%s: %v", s.endpoint, err))
	}
	return "", fmt.Errorf("all paste services failed (%s)", strings.Join(failures, "; "))
}

// uploadTo performs one upload attempt against a single service and returns
// the paste link, or an error covering the HTTP error, timeout, transport or
// malformed-response case.
func uploadTo(ctx context.Context, client *http.Client, s service, data []byte) (string, error) {
	req, err := s.build(data)
	if err != nil {
		return "", fmt.Errorf("failed to build paste request: %w", err)
	}
	req.URL, err = url.Parse(s.endpoint)
	if err != nil {
		return "", fmt.Errorf("invalid paste endpoint %s: %w", s.endpoint, err)
	}
	req.Host = req.URL.Host
	req = req.WithContext(ctx)
	req.Header.Set("User-Agent", "v2pingo")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to upload results to %s: %w", s.endpoint, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return "", fmt.Errorf("failed to read paste response from %s: %w", s.endpoint, err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("paste service %s returned status %d", s.endpoint, resp.StatusCode)
	}

	link := strings.TrimSpace(string(body))
	if !strings.HasPrefix(link, "http://") && !strings.HasPrefix(link, "https://") {
		return "", fmt.Errorf("paste service %s returned an unexpected response: %q", s.endpoint, link)
	}
	return link, nil
}
