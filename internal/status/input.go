package status

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// Input mirrors the JSON that Claude Code writes to the statusline command's
// stdin. Fields documented as "may be absent" are pointers so their absence is
// distinguishable from a legitimate zero value.
type Input struct {
	Cwd           string        `json:"cwd"`
	SessionID     string        `json:"session_id"`
	SessionName   string        `json:"session_name"`
	Version       string        `json:"version"`
	Model         Model         `json:"model"`
	Workspace     Workspace     `json:"workspace"`
	Cost          Cost          `json:"cost"`
	ContextWindow ContextWindow `json:"context_window"`
	Exceeds200k   bool          `json:"exceeds_200k_tokens"`

	Effort     *Effort     `json:"effort"`
	RateLimits *RateLimits `json:"rate_limits"`
	PR         *PR         `json:"pr"`
	Agent      *Agent      `json:"agent"`
	Worktree   *Worktree   `json:"worktree"`
}

type Model struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

type Workspace struct {
	CurrentDir  string `json:"current_dir"`
	ProjectDir  string `json:"project_dir"`
	GitWorktree string `json:"git_worktree"`
}

type Cost struct {
	TotalCostUSD       float64 `json:"total_cost_usd"`
	TotalDurationMs    int64   `json:"total_duration_ms"`
	TotalAPIDurationMs int64   `json:"total_api_duration_ms"`
	TotalLinesAdded    int     `json:"total_lines_added"`
	TotalLinesRemoved  int     `json:"total_lines_removed"`
}

type ContextWindow struct {
	TotalInputTokens  int           `json:"total_input_tokens"`
	TotalOutputTokens int           `json:"total_output_tokens"`
	ContextWindowSize int           `json:"context_window_size"`
	UsedPercentage    *float64      `json:"used_percentage"`
	CurrentUsage      *CurrentUsage `json:"current_usage"`
}

type CurrentUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

type Effort struct {
	Level string `json:"level"`
}

type RateLimits struct {
	FiveHour *Window `json:"five_hour"`
	SevenDay *Window `json:"seven_day"`
}

// Window is one rate-limit bucket. Each may be absent independently.
type Window struct {
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       Epoch   `json:"resets_at"`
}

type PR struct {
	Number      int    `json:"number"`
	URL         string `json:"url"`
	ReviewState string `json:"review_state"`
}

type Agent struct {
	Name string `json:"name"`
}

type Worktree struct {
	Name   string `json:"name"`
	Branch string `json:"branch"`
}

// Epoch is a timestamp that Claude Code sends as Unix epoch seconds. It also
// accepts an RFC3339 string so the same type can decode the OAuth usage API,
// which formats timestamps that way.
type Epoch struct {
	time.Time
	Set bool
}

func (e *Epoch) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if s == "" || s == "null" {
			return nil
		}
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05"} {
			if t, err := time.Parse(layout, s); err == nil {
				e.Time, e.Set = t, true
				return nil
			}
		}
		// A quoted number is still a valid epoch.
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			e.Time, e.Set = time.Unix(n, 0), true
			return nil
		}
		return fmt.Errorf("status: cannot parse timestamp %q", s)
	}
	f, err := strconv.ParseFloat(string(b), 64)
	if err != nil {
		return err
	}
	sec := int64(f)
	e.Time = time.Unix(sec, int64((f-float64(sec))*1e9))
	e.Set = true
	return nil
}

// Parse decodes the statusline JSON payload.
func Parse(b []byte) (Input, error) {
	var in Input
	err := json.Unmarshal(b, &in)
	return in, err
}

// ---- Derived values -------------------------------------------------------

// ContextTokens is the number of tokens currently occupying the context window:
// fresh input plus both halves of the cache. Zero when Claude Code has not yet
// made an API call, since current_usage is null before the first call and again
// after /compact.
func (in Input) ContextTokens() int {
	u := in.ContextWindow.CurrentUsage
	if u == nil {
		return 0
	}
	return u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
}

// ContextSize is the context window size, defaulting to 200k when absent.
func (in Input) ContextSize() int {
	if n := in.ContextWindow.ContextWindowSize; n > 0 {
		return n
	}
	return 200_000
}

// ContextPercent prefers the percentage Claude Code computed, falling back to
// deriving it from the token counts.
func (in Input) ContextPercent() int {
	if p := in.ContextWindow.UsedPercentage; p != nil {
		return clamp(int(*p+0.5), 0, 100)
	}
	if size := in.ContextSize(); size > 0 {
		return clamp(in.ContextTokens()*100/size, 0, 100)
	}
	return 0
}

// CachePercent is the share of input tokens served from the prompt cache. The
// second return value is false when there is no cache activity to report.
func (in Input) CachePercent() (int, bool) {
	u := in.ContextWindow.CurrentUsage
	if u == nil {
		return 0, false
	}
	total := u.InputTokens + u.CacheReadInputTokens
	if total <= 0 || u.CacheReadInputTokens <= 0 {
		return 0, false
	}
	return u.CacheReadInputTokens * 100 / total, true
}

// EffortLevel returns the live reasoning effort, or "" when the model does not
// support the effort parameter.
func (in Input) EffortLevel() string {
	if in.Effort == nil {
		return ""
	}
	return in.Effort.Level
}

// Dir is the working directory, preferring the explicit cwd field.
func (in Input) Dir() string {
	if in.Cwd != "" {
		return in.Cwd
	}
	return in.Workspace.CurrentDir
}
