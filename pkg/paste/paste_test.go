package paste

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "results.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return path
}

func TestUploadFile_PostsBytesAndReturnsLink(t *testing.T) {
	var gotMethod, gotBody, gotContentType string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("https://paste.rs/abc123\n"))
	}))
	defer ts.Close()

	path := writeTemp(t, "vless://user@1.1.1.1:443#node1\n")
	link, err := NewClient(ts.URL).UploadFile(context.Background(), path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if link != "https://paste.rs/abc123" {
		t.Errorf("expected trimmed link, got %q", link)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotBody != "vless://user@1.1.1.1:443#node1\n" {
		t.Errorf("expected raw file bytes as body, got %q", gotBody)
	}
	if !strings.HasPrefix(gotContentType, "text/plain") {
		t.Errorf("expected text/plain content type, got %q", gotContentType)
	}
}

func TestUploadFile_SkipsMissingAndEmptyFiles(t *testing.T) {
	requests := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("https://paste.rs/unexpected"))
	}))
	defer ts.Close()

	client := NewClient(ts.URL)

	missing := filepath.Join(t.TempDir(), "does-not-exist.md")
	link, err := client.UploadFile(context.Background(), missing)
	if err != nil {
		t.Fatalf("missing file must not be an error, got %v", err)
	}
	if link != "" {
		t.Errorf("missing file must return no link, got %q", link)
	}

	empty := writeTemp(t, "  \n\n")
	link, err = client.UploadFile(context.Background(), empty)
	if err != nil {
		t.Fatalf("empty file must not be an error, got %v", err)
	}
	if link != "" {
		t.Errorf("empty file must return no link, got %q", link)
	}

	if requests != 0 {
		t.Errorf("no request should be sent for missing/empty files, got %d", requests)
	}
}

func TestUploadFile_RejectsErrorStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer ts.Close()

	path := writeTemp(t, "vless://user@1.1.1.1:443#node1\n")
	if _, err := NewClient(ts.URL).UploadFile(context.Background(), path); err == nil {
		t.Fatal("expected an error for a non-2xx paste response")
	}
}

func TestUploadFile_RejectsNonURLLink(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("not a link"))
	}))
	defer ts.Close()

	path := writeTemp(t, "vless://user@1.1.1.1:443#node1\n")
	if _, err := NewClient(ts.URL).UploadFile(context.Background(), path); err == nil {
		t.Fatal("expected an error when the paste service does not return a URL")
	}
}

func TestUploadFile_PropagatesTransportError(t *testing.T) {
	path := writeTemp(t, "vless://user@1.1.1.1:443#node1\n")
	// A closed server guarantees the request cannot be delivered.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ts.Close()
	if _, err := NewClient(ts.URL).UploadFile(context.Background(), path); err == nil {
		t.Fatal("expected a transport error for an unreachable paste service")
	}
}

func TestIsAffirmative(t *testing.T) {
	yes := []string{"y", "Y", "yes", "YES", "Yes", "yEs", "  y  ", "\r\ny\n"}
	for _, answer := range yes {
		if !IsAffirmative(answer) {
			t.Errorf("expected %q to be affirmative", answer)
		}
	}

	no := []string{"", " ", "n", "N", "no", "NO", "nope", "yeah", "1", "true", "yy"}
	for _, answer := range no {
		if IsAffirmative(answer) {
			t.Errorf("expected %q to be non-affirmative", answer)
		}
	}
}

func TestPromptsAreDistinctAndNonEmpty(t *testing.T) {
	if strings.TrimSpace(Prompt) == "" || strings.TrimSpace(RetryPrompt) == "" {
		t.Fatal("both the initial and the retry prompt must be non-empty")
	}
	if Prompt == RetryPrompt {
		t.Fatalf("the retry prompt must ask a different question, both were %q", Prompt)
	}
}

func TestNewClient_DefaultsEndpoint(t *testing.T) {
	if got := NewClient("").Endpoint; got != DefaultEndpoint {
		t.Errorf("expected default endpoint %q, got %q", DefaultEndpoint, got)
	}
	if got := NewClient("   ").Endpoint; got != DefaultEndpoint {
		t.Errorf("expected blank endpoint to fall back to default, got %q", got)
	}
	if got := NewClient("https://example.com/").Endpoint; got != "https://example.com/" {
		t.Errorf("expected custom endpoint to be preserved, got %q", got)
	}
}
