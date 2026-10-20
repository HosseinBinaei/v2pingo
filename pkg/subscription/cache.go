package subscription

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// DefaultCacheDir is the project-root directory that keeps a copy of every
// fetched subscription payload so a later run can reuse the same sources
// without hitting the network again.
const DefaultCacheDir = "cache"

// cacheFileSuffix is appended to every cached payload, and maxCacheLabelLen
// bounds the readable part of a cache file name so long URLs stay portable.
const (
	cacheFileSuffix  = ".txt"
	maxCacheLabelLen = 80
)

// cacheFilePattern matches the cache file names written by this package, so
// inspecting or pruning a cache directory never touches unrelated user files.
var cacheFilePattern = regexp.MustCompile(`-[0-9a-f]{12}\.txt$`)

// FetchMode selects where the subscription sources of a run come from.
type FetchMode int

const (
	// FetchNetwork downloads every subscription source and refreshes the cache
	// directory so it mirrors the source list of this run.
	FetchNetwork FetchMode = iota
	// FetchCache reuses the payloads already stored in the cache directory and
	// downloads only the sources that are missing from it.
	FetchCache
)

// CachePrompt is the question asked before any source is fetched. Answering
// y / yes downloads fresh copies from the source links and refreshes the cache;
// every other answer — including Enter and a non-interactive EOF — reuses the
// cached files.
const CachePrompt = "Refresh subscriptions from GitHub? [y/N]: "

// CacheStats describes what a cache directory currently holds.
type CacheStats struct {
	// Dir is the inspected directory.
	Dir string
	// Files is the number of cached subscription payloads found.
	Files int
	// Updated is the modification time of the newest cached payload; it is the
	// zero time when the directory is empty or missing.
	Updated time.Time
}

// CacheFileName returns the deterministic cache file name for a normalized
// subscription URL: a readable host/path label plus a short content address, so
// the payload of a source can be found again on a later run and two sources can
// never collide.
func CacheFileName(normURL string) string {
	digest := sha256.Sum256([]byte(normURL))
	sum := hex.EncodeToString(digest[:])[:12]

	label := ""
	if u, err := url.Parse(normURL); err == nil && u.Host != "" {
		label = sanitizeCacheLabel(u.Host + u.Path)
	}
	if label == "" {
		label = "source"
	}
	return label + "-" + sum + cacheFileSuffix
}

// sanitizeCacheLabel turns a URL host and path into a portable file-name label.
func sanitizeCacheLabel(label string) string {
	var b strings.Builder
	pendingDash := false
	for _, r := range label {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_':
			if pendingDash && b.Len() > 0 {
				b.WriteByte('-')
			}
			pendingDash = false
			b.WriteRune(r)
		default:
			pendingDash = true
		}
		if b.Len() >= maxCacheLabelLen {
			break
		}
	}
	return strings.Trim(b.String(), "-.")
}

// ResolveCacheDir returns the directory used for cached subscription payloads.
// The provided path is used when it already exists, otherwise the directory that
// holds the running executable is checked so a cache created next to
// v2pingo.exe is also found when the tool starts from another working directory.
func ResolveCacheDir(dir string) string {
	if dir == "" {
		dir = DefaultCacheDir
	}
	if _, err := os.Stat(dir); err == nil {
		return dir
	}
	if filepath.IsAbs(dir) {
		return dir
	}
	executable, err := os.Executable()
	if err != nil {
		return dir
	}
	candidate := filepath.Join(filepath.Dir(executable), filepath.Base(dir))
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return dir
}

// InspectCache reports how many subscription payloads dir holds and when the
// newest one was written. A missing or unreadable directory reports zero files.
func InspectCache(dir string) CacheStats {
	stats := CacheStats{Dir: dir}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return stats
	}
	for _, entry := range entries {
		if entry.IsDir() || !cacheFilePattern.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		stats.Files++
		if info.ModTime().After(stats.Updated) {
			stats.Updated = info.ModTime()
		}
	}
	return stats
}

// readCachedSubscription returns the cached payload of normURL, reporting false
// when the source has never been cached or the file is empty.
func readCachedSubscription(dir, normURL string) ([]byte, bool) {
	payload, err := os.ReadFile(cacheFilePath(dir, normURL))
	if err != nil || len(payload) == 0 {
		return nil, false
	}
	return payload, true
}

// writeCachedSubscription stores the payload of normURL in dir, creating the
// directory on the first write.
func writeCachedSubscription(dir, normURL string, payload []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create cache directory %s: %w", dir, err)
	}
	if err := os.WriteFile(cacheFilePath(dir, normURL), payload, 0o644); err != nil {
		return fmt.Errorf("failed to cache subscription %s: %w", normURL, err)
	}
	return nil
}

// pruneCache removes cached payloads that do not belong to keep, so a refreshed
// cache mirrors the sources of the run that just wrote it. Files that this
// package did not write are left untouched.
func pruneCache(dir string, keep map[string]bool) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // nothing was cached yet
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !cacheFilePattern.MatchString(name) || keep[name] {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return fmt.Errorf("failed to remove stale cache file %s: %w", filepath.Join(dir, name), err)
		}
	}
	return nil
}

// cacheFilePath returns the file that stores the payload of normURL.
func cacheFilePath(dir, normURL string) string {
	return filepath.Join(dir, CacheFileName(normURL))
}
