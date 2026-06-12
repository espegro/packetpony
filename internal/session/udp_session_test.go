package session

import (
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func startUDPServer(t *testing.T) *net.UDPConn {
	t.Helper()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("ListenUDP() error = %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestSessionManagerExpiryRemovesOnceAndClosesConnection(t *testing.T) {
	target := startUDPServer(t)
	manager := NewSessionManager(20 * time.Millisecond)
	t.Cleanup(manager.Close)

	var removals atomic.Int32
	removed := make(chan *Session, 1)
	manager.SetRemoveHandler(func(session *Session) {
		removals.Add(1)
		removed <- session
	})

	session, isNew, err := manager.GetOrCreate(
		&net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
		target.LocalAddr().String(),
	)
	if err != nil {
		t.Fatalf("GetOrCreate() error = %v", err)
	}
	if !isNew {
		t.Fatal("expected a new session")
	}

	select {
	case got := <-removed:
		if got != session {
			t.Fatal("remove handler received a different session")
		}
	case <-time.After(time.Second):
		t.Fatal("session was not removed after timeout")
	}

	if manager.Remove(session.ID) != nil {
		t.Fatal("session was removed more than once")
	}
	if removals.Load() != 1 {
		t.Fatalf("remove handler called %d times, want 1", removals.Load())
	}
	if _, err := session.TargetConn.Write([]byte("closed")); err == nil {
		t.Fatal("target connection remained open after removal")
	}
}

func TestSessionManagerCloseInvokesRemoveHandler(t *testing.T) {
	target := startUDPServer(t)
	manager := NewSessionManager(time.Minute)

	removed := make(chan struct{}, 1)
	manager.SetRemoveHandler(func(*Session) {
		removed <- struct{}{}
	})

	if _, _, err := manager.GetOrCreate(
		&net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12346},
		target.LocalAddr().String(),
	); err != nil {
		t.Fatalf("GetOrCreate() error = %v", err)
	}

	manager.Close()
	manager.Close()

	select {
	case <-removed:
	case <-time.After(time.Second):
		t.Fatal("close did not remove the active session")
	}
}
