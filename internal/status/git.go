package status

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// gitTimeout bounds each git invocation. The statusline is re-rendered often,
// so a slow repository must degrade to "no git info" rather than stall the bar.
const gitTimeout = 300 * time.Millisecond

// Repo summarizes the git state of the working directory.
type Repo struct {
	Branch   string // empty when detached or not a repository
	Commit   string // abbreviated HEAD sha
	Added    int    // staged-add and untracked files
	Modified int    // every other tracked change
	Dirty    bool
}

// RepoInfo collects branch, dirty state, and file counts. It returns a zero
// Repo when dir is not a git repository or when git exceeds gitTimeout.
func RepoInfo(ctx context.Context, dir string) Repo {
	if dir == "" {
		return Repo{}
	}
	var r Repo

	// One call yields the branch and the per-file status, halving the fork cost
	// versus asking for the branch separately.
	out, ok := runGit(ctx, dir, "status", "--porcelain=v1", "--branch")
	if !ok {
		return Repo{}
	}
	parseStatus(out, &r)

	if out, ok := runGit(ctx, dir, "rev-parse", "--short", "HEAD"); ok {
		r.Commit = strings.TrimSpace(out)
	}
	return r
}

func runGit(ctx context.Context, dir string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(cmd.Environ(), "GIT_OPTIONAL_LOCKS=0")
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return string(out), true
}

// parseStatus reads `git status --porcelain=v1 --branch` output. The first line
// is the branch header; the rest are two-character status codes plus a path.
func parseStatus(out string, r *Repo) {
	for i, line := range strings.Split(out, "\n") {
		if i == 0 {
			r.Branch = parseBranchHeader(line)
			continue
		}
		if len(line) < 2 {
			continue
		}
		r.Dirty = true
		switch code := line[:2]; {
		case code == "??", code[0] == 'A':
			r.Added++
		default:
			r.Modified++
		}
	}
}

// parseBranchHeader extracts the local branch from the header line of
// `git status --branch`, which takes one of these forms:
//
//	## main...origin/main [ahead 1]   tracking an upstream
//	## main                           no upstream
//	## No commits yet on main         unborn branch, before the first commit
//	## HEAD (no branch)               detached head
func parseBranchHeader(line string) string {
	rest, ok := strings.CutPrefix(line, "## ")
	if !ok {
		return ""
	}
	if strings.HasPrefix(rest, "HEAD (no branch)") {
		return ""
	}
	rest = strings.TrimPrefix(rest, "No commits yet on ")
	if idx := strings.Index(rest, "..."); idx >= 0 {
		rest = rest[:idx]
	}
	if idx := strings.Index(rest, " "); idx >= 0 {
		rest = rest[:idx]
	}
	return strings.TrimSpace(rest)
}
