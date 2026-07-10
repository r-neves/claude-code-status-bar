package status

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// ExtraUsage describes pay-as-you-go credit consumption. Claude Code does not
// send this on stdin, so it is fetched from the OAuth usage API and is only
// populated when the caller opts in.
type ExtraUsage struct {
	Enabled      bool
	Utilization  float64 // 0-100
	UsedCredits  float64 // cents
	MonthlyLimit float64 // cents
}

// BurnRate projects month-end credit spend from the spend so far. It reports
// false on the first of the month, when a single day of data would produce a
// meaningless projection.
func (x ExtraUsage) BurnRate(now time.Time) (daily, projected float64, ok bool) {
	day := now.Day()
	if day < 2 {
		return 0, 0, false
	}
	daily = x.UsedCredits / float64(day)
	daysInMonth := firstOfNextMonth(now).AddDate(0, 0, -1).Day()
	return daily, daily * float64(daysInMonth), true
}

// usageResponse is the subset of the OAuth usage API payload we consume.
type usageResponse struct {
	ExtraUsage struct {
		IsEnabled    bool    `json:"is_enabled"`
		Utilization  float64 `json:"utilization"`
		UsedCredits  float64 `json:"used_credits"`
		MonthlyLimit float64 `json:"monthly_limit"`
	} `json:"extra_usage"`
}

const (
	usageURL      = "https://api.anthropic.com/api/oauth/usage"
	usageCacheTTL = 60 * time.Second
	fetchTimeout  = 2 * time.Second
)

func cachePath() string {
	return filepath.Join(os.TempDir(), "claude-status-bar", "usage-cache.json")
}

// FetchExtraUsage returns credit usage, preferring a cache no older than
// usageCacheTTL. It returns nil rather than an error: the status bar must
// render regardless of network or credential problems.
func FetchExtraUsage(ctx context.Context) *ExtraUsage {
	path := cachePath()

	if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) < usageCacheTTL {
		if x := readCache(path); x != nil {
			return x
		}
	}

	token := oauthToken()
	if token == "" {
		return readCache(path) // fall back to a stale cache
	}

	body, err := requestUsage(ctx, token)
	if err != nil {
		return readCache(path)
	}

	var resp usageResponse
	if json.Unmarshal(body, &resp) != nil {
		return readCache(path)
	}

	// Best effort: a failed cache write only costs a refetch on the next render.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err == nil {
		_ = os.WriteFile(path, body, 0o600)
	}
	return toExtraUsage(resp)
}

func requestUsage(ctx context.Context, token string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, usageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("usage api: unexpected status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

func readCache(path string) *ExtraUsage {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var resp usageResponse
	if json.Unmarshal(data, &resp) != nil {
		return nil
	}
	return toExtraUsage(resp)
}

func toExtraUsage(r usageResponse) *ExtraUsage {
	if !r.ExtraUsage.IsEnabled {
		return nil
	}
	return &ExtraUsage{
		Enabled:      true,
		Utilization:  r.ExtraUsage.Utilization,
		UsedCredits:  r.ExtraUsage.UsedCredits,
		MonthlyLimit: r.ExtraUsage.MonthlyLimit,
	}
}

// oauthToken resolves a Claude Code OAuth token from the environment, the
// platform keychain, or the credentials file, in that order.
func oauthToken() string {
	if t := os.Getenv("CLAUDE_CODE_OAUTH_TOKEN"); t != "" {
		return t
	}
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("security", "find-generic-password",
			"-s", "Claude Code-credentials", "-w").Output(); err == nil {
			if t := accessToken(out); t != "" {
				return t
			}
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		if data, err := os.ReadFile(filepath.Join(home, ".claude", ".credentials.json")); err == nil {
			if t := accessToken(data); t != "" {
				return t
			}
		}
	}
	if runtime.GOOS == "linux" {
		if out, err := exec.Command("secret-tool", "lookup",
			"service", "Claude Code-credentials").Output(); err == nil {
			if t := accessToken(out); t != "" {
				return t
			}
		}
	}
	return ""
}

// accessToken pulls claudeAiOauth.accessToken out of a credentials blob.
func accessToken(data []byte) string {
	var creds struct {
		ClaudeAIOauth struct {
			AccessToken string `json:"accessToken"`
		} `json:"claudeAiOauth"`
	}
	if json.Unmarshal(data, &creds) != nil {
		return ""
	}
	return creds.ClaudeAIOauth.AccessToken
}
