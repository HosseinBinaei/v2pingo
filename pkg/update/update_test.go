package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIsNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"v1.0.0", "v1.0.1", true},
		{"v1.0.0", "v1.1.0", true},
		{"v1.0.0", "v2.0.0", true},
		{"v1.9.0", "v1.10.0", true},  // numeric, not lexicographic
		{"1.0.0", "v1.0.1", true},    // current may lack the v prefix
		{"v1.0.0", "v1.0.0", false},  // equal is not newer
		{"v1.0.1", "v1.0.0", false},  // older latest
		{"v2.0.0", "v1.9.9", false},  // older major
		{"v1.2", "v1.2.0", false},    // missing parts are zero
		{"v1.2", "v1.2.1", true},
		{"v1.2.3-rc1", "v1.2.3", true}, // pre-release suffix is ignored
		{"dev", "v9.9.9", false},       // unparseable current never updates
		{"v1.0.0", "", false},          // empty latest
		{"v1.0.0", "garbage", false},   // unparseable latest
		{"", "v1.0.1", false},
	}
	for _, c := range cases {
		if got := IsNewer(c.current, c.latest); got != c.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestLatest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("missing GitHub Accept header, got %q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name": "v1.2.3", "name": "release"}`))
	}))
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err := Latest(ctx, ts.Client(), ts.URL)
	if err != nil {
		t.Fatalf("Latest failed: %v", err)
	}
	if got != "v1.2.3" {
		t.Errorf("Latest = %q, want v1.2.3", got)
	}
}

func TestLatestErrors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden) // GitHub rate limit
	}))
	defer ts.Close()
	if _, err := Latest(ctx, ts.Client(), ts.URL); err == nil {
		t.Error("a non-200 response must be an error")
	}

	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer broken.Close()
	if _, err := Latest(ctx, broken.Client(), broken.URL); err == nil {
		t.Error("a non-JSON body must be an error")
	}

	if _, err := Latest(ctx, ts.Client(), "http://127.0.0.1:1/nope"); err == nil {
		t.Error("an unreachable endpoint must be an error")
	}
}

func TestNotice(t *testing.T) {
	newer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name": "v9.9.9"}`))
	}))
	defer newer.Close()

	notice := Notice(context.Background(), newer.Client(), "v1.0.0", newer.URL)
	if !strings.Contains(notice, "v9.9.9") || !strings.Contains(notice, "v1.0.0") || !strings.Contains(notice, ReleasesURL) {
		t.Errorf("notice must name the new release, the current one and the releases page, got %q", notice)
	}
	if lines := strings.Count(notice, "\n"); lines != 0 {
		t.Errorf("notice must be a single line, got %d newlines", lines)
	}

	equal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name": "v1.0.0"}`))
	}))
	defer equal.Close()
	if got := Notice(context.Background(), equal.Client(), "v1.0.0", equal.URL); got != "" {
		t.Errorf("a current run must stay silent, got %q", got)
	}
}

func TestNoticeSkipsDevWithoutNetwork(t *testing.T) {
	hits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"tag_name": "v9.9.9"}`))
	}))
	defer ts.Close()

	for _, current := range []string{"dev", "", "  "} {
		if got := Notice(context.Background(), ts.Client(), current, ts.URL); got != "" {
			t.Errorf("current %q must skip the check, got %q", current, got)
		}
	}
	if hits != 0 {
		t.Errorf("dev builds must not touch the network, got %d request(s)", hits)
	}
}

func TestNoticeSilentOnError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	if got := Notice(context.Background(), ts.Client(), "v1.0.0", ts.URL); got != "" {
		t.Errorf("a failed check must stay silent, got %q", got)
	}
	if got := Notice(context.Background(), ts.Client(), "v1.0.0", "http://127.0.0.1:1/nope"); got != "" {
		t.Errorf("an unreachable endpoint must stay silent, got %q", got)
	}
}
