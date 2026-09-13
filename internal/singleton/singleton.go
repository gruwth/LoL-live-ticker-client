// Package singleton keeps exactly one agent running per machine, and doubles
// as the channel that tells the running one to show its window.
//
// This matters more than it looks. The relay closes the older connection when
// a second one authenticates with the same token, so two agents fight: each
// reconnects, each kicks the other, forever. As a CLI that was unlikely; with
// a tray icon and auto-start it becomes routine, because the user
// double-clicks the icon while the app is already running.
//
// A loopback listener is used rather than a PID lockfile: a lockfile goes
// stale on a crash and then needs liveness checking, whereas a socket is
// released by the OS when the process dies, however it dies.
package singleton

import (
	"errors"
	"fmt"
	"net"
	"time"
)

// Port is the fixed loopback port the primary instance binds. It is
// deliberately hardcoded and documented rather than negotiated: both sides
// need to agree on it without any shared state on disk.
const Port = 47821

// showByte is the entire protocol. Anything else is ignored.
const showByte = 'S'

const dialTimeout = 2 * time.Second

// ErrAlreadyRunning means another instance holds the lock.
var ErrAlreadyRunning = errors.New("another instance is already running")

// Lock is the primary instance's hold on the machine.
type Lock struct {
	ln net.Listener
}

// Acquire binds the loopback port. It returns ErrAlreadyRunning if another
// instance already has it, in which case the caller should Show and exit.
func Acquire() (*Lock, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", Port))
	if err != nil {
		if isAddrInUse(err) {
			return nil, ErrAlreadyRunning
		}
		return nil, err
	}
	return &Lock{ln: ln}, nil
}

// Serve calls onShow every time another instance asks for the window. It
// blocks until the lock is closed, so run it in a goroutine.
func (l *Lock) Serve(onShow func()) {
	for {
		conn, err := l.ln.Accept()
		if err != nil {
			return // listener closed
		}
		go l.handle(conn, onShow)
	}
}

func (l *Lock) handle(conn net.Conn, onShow func()) {
	defer conn.Close()

	// Only loopback may talk to us. The listener is already bound to
	// 127.0.0.1, so this is belt and braces against a stray route.
	if host, _, err := net.SplitHostPort(conn.RemoteAddr().String()); err == nil {
		if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			return
		}
	}

	conn.SetReadDeadline(time.Now().Add(dialTimeout))
	var b [1]byte
	if n, err := conn.Read(b[:]); err != nil || n != 1 || b[0] != showByte {
		return // not our protocol; ignore rather than guess
	}
	if onShow != nil {
		onShow()
	}
}

// Close releases the lock.
func (l *Lock) Close() error { return l.ln.Close() }

// Show tells the instance that already holds the lock to raise its window.
// It is called by the losing instance just before it exits.
func Show() error {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", Port), dialTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetWriteDeadline(time.Now().Add(dialTimeout))
	_, err = conn.Write([]byte{showByte})
	return err
}
