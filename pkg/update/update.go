// Package update implements the one-line "a newer release exists" notice.
// The whole feature is best-effort: every failure mode — offline, rate
// limited, unreachable API — results in silence, never in an error shown to
// the user, so a run is never disturbed by the check.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// Repo is the GitHub repository releases are published under.
	Repo = "HosseinBinaei/v2pingo"
	// ReleasesURL is the human-visible page of the latest release.
	ReleasesURL = "https://github.com/" + Repo + "/releases/latest"
	// APIURL is the GitHub endpoint Latest queries by default.
	APIURL = "https://api.github.com/repos/" + Repo + "/releases/latest"
	// Timeout bounds a single check; the caller wraps it in a context too.
	Timeout = 3 * time.Second
	// acceptHeader pins the GitHub REST media type.
	acceptHeader = "application/vnd.github+json"
)

// Latest returns the tag of the newest published release, e.g. "v1.2.3".
// An empty endpoint means the real GitHub API; a nil client means
// http.DefaultClient. The caller's context bounds the request.
func Latest(ctx context.Context, client *http.Client, endpoint string) (string, error) {
	if endpoint == "" {
		endpoint = APIURL
	}
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", acceptHeader)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github api: %s", resp.Status)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&payload); err != nil {
		return "", err
	}
	return strings.TrimSpace(payload.TagName), nil
}

// IsNewer reports whether latest is a strictly newer release than current.
// Tags are compared numerically part by part after stripping the v prefix
// (v1.10.0 > v1.9.0), missing parts count as zero, and a pre-release
// (v1.2.3-rc1) is older than the release it belongs to. Tags that cannot be
// parsed never report an update.
func IsNewer(current, latest string) bool {
	c, okCurrent := semverParts(current)
	l, okLatest := semverParts(latest)
	if !okCurrent || !okLatest {
		return false
	}
	for i := range l.nums {
		if l.nums[i] != c.nums[i] {
			return l.nums[i] > c.nums[i]
		}
	}
	// Numerically equal: the plain release outranks its pre-releases.
	return c.pre && !l.pre
}

// Notice returns the single-line update notice for a run of version current,
// or "" when the check is skipped (dev or unknown version), the run is
// already current, or the probe fails for any reason.
func Notice(ctx context.Context, client *http.Client, current, endpoint string) string {
	current = strings.TrimSpace(current)
	if current == "" || current == "dev" {
		return ""
	}
	latest, err := Latest(ctx, client, endpoint)
	if err != nil || !IsNewer(current, latest) {
		return ""
	}
	return fmt.Sprintf("Update available: %s (you are running %s) — %s", latest, current, ReleasesURL)
}

// semver holds the parsed numeric parts of a release tag; pre records a
// -suffix (pre-release).
type semver struct {
	nums [3]int
	pre  bool
}

func semverParts(tag string) (semver, bool) {
	var s semver
	tag = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(tag), "v"), "V")
	if i := strings.IndexAny(tag, "-+"); i >= 0 {
		s.pre = tag[i] == '-'
		tag = tag[:i]
	}
	nums := strings.Split(tag, ".")
	if len(nums) == 0 || len(nums) > 3 {
		return s, false
	}
	for i, n := range nums {
		v, err := strconv.Atoi(n)
		if err != nil || v < 0 {
			return s, false
		}
		s.nums[i] = v
	}
	return s, true
}
