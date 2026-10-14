package subscription

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// linkServer serves payload as a subscription source and counts every request so
// a test can prove whether the network was touched.
func linkServer(t *testing.T, payload *string, hits *int) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits != nil {
			*hits++
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(*payload))
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestCacheFileName(t *testing.T) {
	source := "https://raw.githubusercontent.com/user/repo/main/all.txt"
	name := CacheFileName(source)

	if name != CacheFileName(source) {
		t.Fatalf("cache file name must be deterministic, got %q then %q", name, CacheFileName(source))
	}
	if name == CacheFileName("https://raw.githubusercontent.com/user/repo/main/other.txt") {
		t.Fatalf("different sources must not share a cache file, both got %q", name)
	}
	if !strings.HasSuffix(name, cacheFileSuffix) {
		t.Errorf("cache file name %q must end in %q", name, cacheFileSuffix)
	}
	if !cacheFilePattern.MatchString(name) {
		t.Errorf("cache file name %q must be recognised as a cache entry", name)
	}
	if strings.ContainsAny(name, `/\:*?"<>|`) {
		t.Errorf("cache file name %q must stay a portable file name", name)
	}
	if !strings.Contains(name, "repo") {
		t.Errorf("cache file name %q should keep a readable source label", name)
	}

	// A host-less source still yields a recognisable cache file, so inspecting
	// and pruning keep working for every name this package writes.
	if hostless := CacheFileName("weird-source"); !cacheFilePattern.MatchString(hostless) {
		t.Errorf("cache file name %q for a host-less source must stay recognisable", hostless)
	}
}

func TestInspectCacheCountsOnlyCachedPayloads(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if stats := InspectCache(missing); stats.Files != 0 || !stats.Updated.IsZero() {
		t.Fatalf("a missing cache directory must report nothing cached, got %+v", stats)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("not a cache entry"), 0644); err != nil {
		t.Fatalf("failed to write unrelated file: %v", err)
	}
	if stats := InspectCache(dir); stats.Files != 0 {
		t.Fatalf("unrelated files must not be counted as cached sources, got %+v", stats)
	}

	url := "https://example.com/one.txt"
	if err := os.WriteFile(filepath.Join(dir, CacheFileName(url)), []byte("vless://a@1.1.1.1:443#a"), 0644); err != nil {
		t.Fatalf("failed to write cached payload: %v", err)
	}
	stats := InspectCache(dir)
	if stats.Files != 1 {
		t.Fatalf("expected one cached source, got %+v", stats)
	}
	if stats.Updated.IsZero() || time.Since(stats.Updated) > time.Hour {
		t.Fatalf("expected a fresh cache timestamp, got %v", stats.Updated)
	}
}

func TestFetchSubscriptionsCacheRefreshesAndPrunes(t *testing.T) {
	one := "vless://cached1@1.1.1.1:443#One"
	two := "trojan://cached2@2.2.2.2:443#Two"
	ts1 := linkServer(t, &one, nil)
	ts2 := linkServer(t, &two, nil)

	dir := filepath.Join(t.TempDir(), "cache")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	links := FetchSubscriptions(ctx, []string{ts1.URL, ts2.URL}, nil, dir, FetchNetwork)
	if len(links) != 2 {
		t.Fatalf("expected 2 links after a fresh download, got %d: %v", len(links), links)
	}
	if stats := InspectCache(dir); stats.Files != 2 {
		t.Fatalf("expected both sources cached, got %+v", stats)
	}
	if payload, ok := readCachedSubscription(dir, ts1.URL); !ok || string(payload) != one {
		t.Fatalf("cached payload must match the downloaded body, got %q (ok=%v)", payload, ok)
	}

	// Dropping a source from the list must remove its cached payload, so the
	// refreshed cache mirrors the sources of the latest run.
	links = FetchSubscriptions(ctx, []string{ts1.URL}, nil, dir, FetchNetwork)
	if len(links) != 1 {
		t.Fatalf("expected 1 link after the refresh, got %d: %v", len(links), links)
	}
	stats := InspectCache(dir)
	if stats.Files != 1 {
		t.Fatalf("expected the dropped source to be pruned, got %+v", stats)
	}
	if _, err := os.Stat(filepath.Join(dir, CacheFileName(ts1.URL))); err != nil {
		t.Fatalf("the remaining source must stay cached: %v", err)
	}
	// A user file in the cache directory is never removed.
	userFile := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(userFile, []byte("keep me"), 0644); err != nil {
		t.Fatalf("failed to write unrelated file: %v", err)
	}
	FetchSubscriptions(ctx, []string{ts1.URL}, nil, dir, FetchNetwork)
	if _, err := os.Stat(userFile); err != nil {
		t.Fatalf("pruning must not touch unrelated files: %v", err)
	}
}

func TestFetchSubscriptionsCacheReusesCacheOffline(t *testing.T) {
	payload := "vless://offline@1.1.1.1:443#Offline"
	hits := 0
	ts := linkServer(t, &payload, &hits)

	dir := filepath.Join(t.TempDir(), "cache")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if links := FetchSubscriptions(ctx, []string{ts.URL}, nil, dir, FetchNetwork); len(links) != 1 {
		t.Fatalf("expected the source to be downloaded and cached, got %v", links)
	}

	// With the source gone, only the cached payload can produce the link.
	ts.Close()
	hitsBefore := hits
	links := FetchSubscriptions(ctx, []string{ts.URL}, nil, dir, FetchCache)
	if len(links) != 1 || links[0] != payload {
		t.Fatalf("expected the cached payload to be reused offline, got %v", links)
	}
	if hits != hitsBefore {
		t.Fatalf("cache mode must not refetch a cached source (hits %d -> %d)", hitsBefore, hits)
	}
}

func TestFetchSubscriptionsCacheDownloadsMissingSources(t *testing.T) {
	cached := "vless://cached@1.1.1.1:443#Cached"
	fresh := "vless://fresh@2.2.2.2:443#Fresh"
	tsCached := linkServer(t, &cached, nil)

	dir := filepath.Join(t.TempDir(), "cache")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if links := FetchSubscriptions(ctx, []string{tsCached.URL}, nil, dir, FetchNetwork); len(links) != 1 {
		t.Fatalf("expected the first source to be cached, got %v", links)
	}

	tsFresh := linkServer(t, &fresh, nil)
	links := FetchSubscriptions(ctx, []string{tsCached.URL, tsFresh.URL}, nil, dir, FetchCache)
	if len(links) != 2 {
		t.Fatalf("expected the missing source to be downloaded, got %d links: %v", len(links), links)
	}
	if stats := InspectCache(dir); stats.Files != 2 {
		t.Fatalf("the downloaded source must be added to the cache, got %+v", stats)
	}
}

func TestFetchSubscriptionsCacheKeepsCachedCopyOnFailure(t *testing.T) {
	payload := "vless://keepme@1.1.1.1:443#Keep"
	ts := linkServer(t, &payload, nil)

	dir := filepath.Join(t.TempDir(), "cache")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if links := FetchSubscriptions(ctx, []string{ts.URL}, nil, dir, FetchNetwork); len(links) != 1 {
		t.Fatalf("expected the source to be cached first, got %v", links)
	}

	// A failed refresh keeps the previous payload so a later run can still use it.
	ts.Close()
	if links := FetchSubscriptions(ctx, []string{ts.URL}, nil, dir, FetchNetwork); len(links) != 0 {
		t.Fatalf("expected no links from an unreachable source, got %v", links)
	}
	if stats := InspectCache(dir); stats.Files != 1 {
		t.Fatalf("the cached payload of a failed source must survive, got %+v", stats)
	}
	links := FetchSubscriptions(ctx, []string{ts.URL}, nil, dir, FetchCache)
	if len(links) != 1 || links[0] != payload {
		t.Fatalf("expected the kept payload to be reusable, got %v", links)
	}
}

// TestFetchSubscriptionsWithoutCacheDirWritesNothing proves the opt-out: an empty
// cache directory downloads the sources and leaves no cache behind.
func TestFetchSubscriptionsWithoutCacheDirWritesNothing(t *testing.T) {
	payload := "vless://nocache@1.1.1.1:443#NoCache"
	ts := linkServer(t, &payload, nil)

	dir := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to read the working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to enter the temp directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("failed to restore the working directory: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if links := FetchSubscriptions(ctx, []string{ts.URL}, nil, "", FetchNetwork); len(links) != 1 {
		t.Fatalf("expected the plain fetch to return the link, got %v", links)
	}
	if stats := InspectCache(DefaultCacheDir); stats.Files != 0 {
		t.Fatalf("a fetch without a cache directory must not write one, got %+v", stats)
	}
}

func TestResolveCacheDir(t *testing.T) {
	existing := t.TempDir()
	if got := ResolveCacheDir(existing); got != existing {
		t.Errorf("ResolveCacheDir(existing) = %q, want %q", got, existing)
	}

	missing := filepath.Join(existing, "nested", "cache")
	if got := ResolveCacheDir(missing); got != missing {
		t.Errorf("ResolveCacheDir(absolute missing) = %q, want %q", got, missing)
	}

	if got := ResolveCacheDir(""); got != DefaultCacheDir {
		t.Errorf("ResolveCacheDir(\"\") = %q, want %q", got, DefaultCacheDir)
	}
}
