//go:build darwin || linux || freebsd || netbsd || openbsd

package status

import (
	"os"
	"syscall"
	"unsafe"
)

// winsize matches the struct filled in by ioctl(2) TIOCGWINSZ.
type winsize struct {
	rows, cols, xpixel, ypixel uint16
}

// ttyWidth asks the controlling terminal for its column count.
func ttyWidth() (int, bool) {
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return 0, false
	}
	defer tty.Close()

	var ws winsize
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		tty.Fd(),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(&ws)),
	)
	if errno != 0 {
		return 0, false
	}
	return int(ws.cols), true
}
