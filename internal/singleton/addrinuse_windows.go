//go:build windows

package singleton

import (
	"errors"
	"syscall"
)

// Winsock reports these as WSA* codes in the 10000 range, which do not match
// the Unix-shaped syscall.EADDRINUSE constant that Go also defines on Windows.
// Comparing against the constant alone silently never matches, so the numbers
// are spelled out.
const (
	wsaeacces     = syscall.Errno(10013) // permission denied: someone else holds the exact address
	wsaeaddrinuse = syscall.Errno(10048) // address already in use
)

func isAddrInUse(err error) bool {
	return errors.Is(err, wsaeaddrinuse) ||
		errors.Is(err, wsaeacces) ||
		errors.Is(err, syscall.EADDRINUSE)
}
