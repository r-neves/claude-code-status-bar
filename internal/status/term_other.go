//go:build !(darwin || linux || freebsd || netbsd || openbsd)

package status

// ttyWidth has no portable implementation on this platform, so callers fall
// back to COLUMNS or DefaultWidth.
func ttyWidth() (int, bool) { return 0, false }
