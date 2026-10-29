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
	"time"
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

// onlyPrimary returns a client that talks to endpoint alone, with the default
// fallback services disabled. Existing single-service behavior tests use it so
// they never touch the network.
func onlyPrimary(t *testing.T, endpoint string) *Client {
	t.Helper()
	c := NewClient(endpoint)
	c.fallbacks = []service{}
	return c
}

func TestUploadFile_RejectsErrorStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer ts.Close()

	path := writeTemp(t, "vless://user@1.1.1.1:443#node1\n")
	if _, err := onlyPrimary(t, ts.URL).UploadFile(context.Background(), path); err == nil {
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
	if _, err := onlyPrimary(t, ts.URL).UploadFile(context.Background(), path); err == nil {
		t.Fatal("expected an error when the paste service does not return a URL")
	}
}

func TestUploadFile_PropagatesTransportError(t *testing.T) {
	path := writeTemp(t, "vless://user@1.1.1.1:443#node1\n")
	// A closed server guarantees the request cannot be delivered.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ts.Close()
	if _, err := onlyPrimary(t, ts.URL).UploadFile(context.Background(), path); err == nil {
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

const sampleResults = "vless://user@1.1.1.1:443#node1\nvless://user@2.2.2.2:443#node2\n"

// statusServer returns a paste-like server that always answers with code and
// body, counting how often it was hit.
func statusServer(t *testing.T, code int, body string, hits *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			*hits++
		}
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}))
}

func TestUploadFile_FallsBackAfterHTTP400(t *testing.T) {
	primaryHits := 0
	primary := statusServer(t, http.StatusBadRequest,
		"400: Bad Request", &primaryHits)
	defer primary.Close()
	fallbackHits := 0
	fallback := statusServer(t, http.StatusCreated, "https://fallback.test/ok", &fallbackHits)
	defer fallback.Close()

	path := writeTemp(t, sampleResults)
	c := NewClient(primary.URL)
	c.fallbacks = []service{{endpoint: fallback.URL, build: rawBodyRequest}}

	link, err := c.UploadFile(context.Background(), path)
	if err != nil {
		t.Fatalf("expected the fallback service to succeed after a 400, got %v", err)
	}
	if link != "https://fallback.test/ok" {
		t.Errorf("expected the fallback link, got %q", link)
	}
	if primaryHits != 1 || fallbackHits != 1 {
		t.Errorf("expected one request to each service, got primary=%d fallback=%d",
			primaryHits, fallbackHits)
	}
}

func TestUploadFile_FallbackOrderUntilSuccess(t *testing.T) {
	secondHits, thirdHits := 0, 0
	primary := statusServer(t, http.StatusInternalServerError, "boom", nil)
	defer primary.Close()
	secondSrv := statusServer(t, http.StatusBadRequest, "bad", &secondHits)
	defer secondSrv.Close()
	third := statusServer(t, http.StatusCreated, "https://third.test/ok", &thirdHits)
	defer third.Close()

	path := writeTemp(t, sampleResults)
	c := NewClient(primary.URL)
	c.fallbacks = []service{
		{endpoint: secondSrv.URL, build: rawBodyRequest},
		{endpoint: third.URL, build: rawBodyRequest},
	}

	link, err := c.UploadFile(context.Background(), path)
	if err != nil {
		t.Fatalf("expected the last service to succeed, got %v", err)
	}
	if link != "https://third.test/ok" {
		t.Errorf("expected the third service's link, got %q", link)
	}
	if thirdHits != 1 || secondHits != 1 {
		t.Errorf("expected services tried in order once each, got second=%d third=%d",
			secondHits, thirdHits)
	}
}

func TestUploadFile_TimeoutFallsBack(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("https://slow.test/too-late"))
	}))
	defer slow.Close()
	fallbackHits := 0
	fallback := statusServer(t, http.StatusCreated, "https://fast.test/ok", &fallbackHits)
	defer fallback.Close()

	path := writeTemp(t, sampleResults)
	c := NewClient(slow.URL)
	c.HTTP = &http.Client{Timeout: 50 * time.Millisecond}
	c.fallbacks = []service{{endpoint: fallback.URL, build: rawBodyRequest}}

	link, err := c.UploadFile(context.Background(), path)
	if err != nil {
		t.Fatalf("expected a timeout on the primary to fall back, got %v", err)
	}
	if link != "https://fast.test/ok" || fallbackHits != 1 {
		t.Errorf("expected the fallback to answer, got link %q hits %d", link, fallbackHits)
	}
}

func TestUploadFile_ConnectivityErrorFallsBack(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	dead.Close() // nothing listens on this port anymore
	fallbackHits := 0
	fallback := statusServer(t, http.StatusCreated, "https://alive.test/ok", &fallbackHits)
	defer fallback.Close()

	path := writeTemp(t, sampleResults)
	c := NewClient(dead.URL)
	c.fallbacks = []service{{endpoint: fallback.URL, build: rawBodyRequest}}

	link, err := c.UploadFile(context.Background(), path)
	if err != nil {
		t.Fatalf("expected a connectivity failure on the primary to fall back, got %v", err)
	}
	if link != "https://alive.test/ok" || fallbackHits != 1 {
		t.Errorf("expected the fallback to answer, got link %q hits %d", link, fallbackHits)
	}
}

func TestUploadFile_AllServicesFail(t *testing.T) {
	primary := statusServer(t, http.StatusBadRequest, "bad", nil)
	defer primary.Close()
	a := statusServer(t, http.StatusInternalServerError, "boom", nil)
	defer a.Close()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	b.Close() // connectivity failure

	path := writeTemp(t, sampleResults)
	c := NewClient(primary.URL)
	c.fallbacks = []service{
		{endpoint: a.URL, build: rawBodyRequest},
		{endpoint: b.URL, build: rawBodyRequest},
	}

	link, err := c.UploadFile(context.Background(), path)
	if err == nil {
		t.Fatalf("expected an error when every service fails, got link %q", link)
	}
	if link != "" {
		t.Errorf("a failed upload must not return a link, got %q", link)
	}
	for _, endpoint := range []string{primary.URL, a.URL, b.URL} {
		if !strings.Contains(err.Error(), endpoint) {
			t.Errorf("expected the error to mention every tried endpoint, missing %q in %q",
				endpoint, err.Error())
		}
	}
}

func TestUploadFile_FirstServiceSuccessSkipsFallbacks(t *testing.T) {
	primaryHits := 0
	primary := statusServer(t, http.StatusCreated, "https://primary.test/ok", &primaryHits)
	defer primary.Close()
	fallbackHits := 0
	fallback := statusServer(t, http.StatusCreated, "https://fallback.test/ok", &fallbackHits)
	defer fallback.Close()

	path := writeTemp(t, sampleResults)
	c := NewClient(primary.URL)
	c.fallbacks = []service{{endpoint: fallback.URL, build: rawBodyRequest}}

	link, err := c.UploadFile(context.Background(), path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if link != "https://primary.test/ok" {
		t.Errorf("expected the primary link, got %q", link)
	}
	if fallbackHits != 0 {
		t.Errorf("a successful primary upload must not touch the fallbacks, got %d calls",
			fallbackHits)
	}
	if primaryHits != 1 {
		t.Errorf("expected exactly one primary request, got %d", primaryHits)
	}
}

func TestUploadFile_DpasteFallbackSendsFormContent(t *testing.T) {
	var gotMethod, gotContentType, gotContent, gotLexer, gotFormat string
	dpaste := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		if err := r.ParseForm(); err != nil {
			t.Errorf("failed to parse the dpaste form body: %v", err)
		}
		gotContent = r.PostFormValue("content")
		gotLexer = r.PostFormValue("lexer")
		gotFormat = r.PostFormValue("format")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("https://dpaste.example/xyz\n"))
	}))
	defer dpaste.Close()

	path := writeTemp(t, sampleResults)
	c := NewClient("https://primary.test/unreachable")
	c.fallbacks = []service{{endpoint: dpaste.URL, build: dpasteRequest}}

	link, err := c.UploadFile(context.Background(), path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if link != "https://dpaste.example/xyz" {
		t.Errorf("expected the dpaste link, got %q", link)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %s", gotMethod)
	}
	if gotContent != sampleResults {
		t.Errorf("expected the raw file contents in the content field, got %q", gotContent)
	}
	if gotLexer != "text" {
		t.Errorf("expected lexer=text, got %q", gotLexer)
	}
	if gotFormat != "url" {
		t.Errorf("expected format=url, got %q", gotFormat)
	}
	if !strings.HasPrefix(gotContentType, "application/x-www-form-urlencoded") {
		t.Errorf("expected a form-encoded body, got content type %q", gotContentType)
	}
}

func TestUploadFile_DuplicateFallbackEndpointNotRetried(t *testing.T) {
	hits := 0
	both := statusServer(t, http.StatusBadRequest, "bad", &hits)
	defer both.Close()

	path := writeTemp(t, sampleResults)
	c := NewClient(both.URL)
	c.fallbacks = []service{{endpoint: both.URL, build: rawBodyRequest}}

	_, err := c.UploadFile(context.Background(), path)
	if err == nil {
		t.Fatal("expected an error when the only service keeps failing")
	}
	if hits != 1 {
		t.Errorf("expected the same endpoint to be tried once, got %d requests", hits)
	}
}

func TestNewClient_CustomEndpointDisablesFallbacks(t *testing.T) {
	path := writeTemp(t, sampleResults)
	// A closed server guarantees nothing can answer; if fallbacks were still
	// active the client would reach the real paste.c-net.org/dpaste.org.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ts.Close()

	if _, err := NewClient(ts.URL).UploadFile(context.Background(), path); err == nil {
		t.Fatal("an explicitly configured endpoint must be used alone, without fallbacks")
	}
}
