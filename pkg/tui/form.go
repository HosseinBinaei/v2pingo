package tui

import (
	"strconv"
	"strings"
	"time"
)

// FormConfig is the validated TUI form output handed to the runner.
type FormConfig struct {
	Subs      []string
	TargetURL string
	Workers   int
	Timeout   time.Duration
}

// ValidateForm checks raw form strings. It returns the config and "" on
// success, or an empty config plus a human-readable error message.
func ValidateForm(subs []string, extraSub, target, workersStr, timeoutStr string) (FormConfig, string) {
	all := make([]string, 0, len(subs)+1)
	for _, s := range subs {
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			all = append(all, trimmed)
		}
	}
	if trimmed := strings.TrimSpace(extraSub); trimmed != "" {
		all = append(all, trimmed)
	}
	if len(all) == 0 {
		return FormConfig{}, "at least one subscription URL is required"
	}
	if strings.TrimSpace(target) == "" {
		return FormConfig{}, "target URL must not be empty"
	}
	w, err := strconv.Atoi(strings.TrimSpace(workersStr))
	if err != nil || w < 1 || w > 200 {
		return FormConfig{}, "workers must be a number between 1 and 200"
	}
	d, err := time.ParseDuration(strings.TrimSpace(timeoutStr))
	if err != nil || d <= 0 {
		return FormConfig{}, "timeout must be a valid duration like 15s"
	}
	return FormConfig{Subs: all, TargetURL: strings.TrimSpace(target), Workers: w, Timeout: d}, ""
}
