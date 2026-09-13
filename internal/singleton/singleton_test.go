package singleton

import (
	"errors"
	"fmt"
	"net"
	"testing"
	"time"
)

func dial() (net.Conn, error) {
	return net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", Port), dialTimeout)
}

func TestSecondInstanceIsRefusedAndCanShowTheFirst(t *testing.T) {
	first, err := Acquire()
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	defer first.Close()

	shown := make(chan struct{}, 1)
	go first.Serve(func() { shown <- struct{}{} })

	if _, err := Acquire(); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second Acquire = %v, want ErrAlreadyRunning", err)
	}

	if err := Show(); err != nil {
		t.Fatalf("Show: %v", err)
	}
	select {
	case <-shown:
	case <-time.After(3 * time.Second):
		t.Fatal("the running instance was never asked to show its window")
	}
}

// Releasing the lock must let the next instance take it, however the previous
// one went away.
func TestLockIsReleasedOnClose(t *testing.T) {
	first, err := Acquire()
	if err != nil {
		t.Fatal(err)
	}
	first.Close()

	second, err := Acquire()
	if err != nil {
		t.Fatalf("Acquire after Close: %v", err)
	}
	second.Close()
}

// Garbage on the port must not be mistaken for a show request.
func TestJunkIsIgnored(t *testing.T) {
	l, err := Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	shown := make(chan struct{}, 1)
	go l.Serve(func() { shown <- struct{}{} })

	conn, err := dial()
	if err != nil {
		t.Fatal(err)
	}
	conn.Write([]byte{'X'})
	conn.Close()

	select {
	case <-shown:
		t.Fatal("a stray byte was treated as a show request")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestShowWithNobodyListening(t *testing.T) {
	if err := Show(); err == nil {
		t.Error("Show succeeded with no instance running, want an error")
	}
}
