package listener

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/espegro/packetpony/internal/config"
	"github.com/espegro/packetpony/internal/logging"
	"github.com/espegro/packetpony/internal/metrics"
)

type discardLogger struct{}

func (discardLogger) LogConnection(logging.ConnectionEvent)     {}
func (discardLogger) LogError(string, map[string]interface{})   {}
func (discardLogger) LogInfo(string, map[string]interface{})    {}
func (discardLogger) LogWarning(string, map[string]interface{}) {}
func (discardLogger) Close() error                              { return nil }

var (
	metricsOnce sync.Once
	testMetrics *metrics.ProxyMetrics
)

func sharedTestMetrics() *metrics.ProxyMetrics {
	metricsOnce.Do(func() {
		testMetrics = metrics.NewProxyMetrics()
	})
	return testMetrics
}

func TestTCPListenerStopAllowsActiveConnectionToFinish(t *testing.T) {
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("target Listen() error = %v", err)
	}
	defer target.Close()

	targetDone := make(chan struct{})
	go func() {
		defer close(targetDone)
		conn, acceptErr := target.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 1024)
		for {
			n, readErr := conn.Read(buf)
			if n > 0 {
				if _, writeErr := conn.Write(buf[:n]); writeErr != nil {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}()

	cfg := &config.ListenerConfig{
		Name:          "test",
		Protocol:      "tcp",
		ListenAddress: "127.0.0.1:0",
		TargetAddress: target.Addr().String(),
		Allowlist:     []string{"127.0.0.1"},
	}
	listener, err := NewTCPListener(t.Context(), cfg, discardLogger{}, sharedTestMetrics())
	if err != nil {
		t.Fatalf("NewTCPListener() error = %v", err)
	}
	if err := listener.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	client, err := net.Dial("tcp", listener.listener.Addr().String())
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}

	stopDone := make(chan error, 1)
	go func() {
		stopDone <- listener.Stop()
	}()

	client.SetDeadline(time.Now().Add(time.Second))
	if _, err := client.Write([]byte("still-open")); err != nil {
		t.Fatalf("active connection write failed during graceful stop: %v", err)
	}
	buf := make([]byte, len("still-open"))
	if _, err := io.ReadFull(client, buf); err != nil {
		t.Fatalf("active connection read failed during graceful stop: %v", err)
	}

	select {
	case err := <-stopDone:
		t.Fatalf("Stop() returned before active connection closed: %v", err)
	default:
	}

	tcpClient := client.(*net.TCPConn)
	if err := tcpClient.CloseWrite(); err != nil {
		t.Fatalf("CloseWrite() error = %v", err)
	}
	if _, err := io.Copy(io.Discard, client); err != nil {
		t.Fatalf("waiting for target EOF failed: %v", err)
	}
	client.Close()
	select {
	case err := <-stopDone:
		if err != nil {
			t.Fatalf("Stop() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Stop() did not finish after active connection closed")
	}
	<-targetDone
}

func TestManagerReloadPreservesExistingTCPConnections(t *testing.T) {
	targetA := startTaggedTCPServer(t, "A:")
	targetB := startTaggedTCPServer(t, "B:")
	listenAddress := reserveTCPAddress(t)

	initial := testManagerConfig(listenAddress, targetA)
	manager, err := NewManager(initial, discardLogger{}, sharedTestMetrics())
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	if err := manager.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		_ = manager.GracefulShutdown(time.Second)
	})

	first, err := net.Dial("tcp", listenAddress)
	if err != nil {
		t.Fatalf("first Dial() error = %v", err)
	}
	defer first.Close()
	assertTaggedResponse(t, first, "before", "A:before")

	reloaded := testManagerConfig(listenAddress, targetB)
	if err := manager.Reload(reloaded); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	second, err := net.Dial("tcp", listenAddress)
	if err != nil {
		t.Fatalf("second Dial() error = %v", err)
	}
	defer second.Close()
	assertTaggedResponse(t, second, "new", "B:new")

	assertTaggedResponse(t, first, "after", "A:after")
}

func testManagerConfig(listenAddress, targetAddress string) *config.Config {
	return &config.Config{
		Server:  config.ServerConfig{Name: "test", ShutdownTimeout: time.Second},
		Logging: config.LoggingConfig{Stdout: config.StdoutConfig{Enabled: true}},
		Listeners: []config.ListenerConfig{{
			Name:          "reload-test",
			Protocol:      "tcp",
			ListenAddress: listenAddress,
			TargetAddress: targetAddress,
			Allowlist:     []string{"127.0.0.1"},
		}},
	}
}

func reserveTCPAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve Listen() error = %v", err)
	}
	address := listener.Addr().String()
	listener.Close()
	return address
}

func startTaggedTCPServer(t *testing.T, prefix string) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("target Listen() error = %v", err)
	}
	t.Cleanup(func() { listener.Close() })

	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				buffer := make([]byte, 128)
				for {
					n, readErr := conn.Read(buffer)
					if n > 0 {
						_, _ = conn.Write(append([]byte(prefix), buffer[:n]...))
					}
					if readErr != nil {
						return
					}
				}
			}(conn)
		}
	}()
	return listener.Addr().String()
}

func assertTaggedResponse(t *testing.T, conn net.Conn, request, expected string) {
	t.Helper()
	if err := conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetDeadline() error = %v", err)
	}
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatalf("Write(%q) error = %v", request, err)
	}
	response := make([]byte, len(expected))
	if _, err := io.ReadFull(conn, response); err != nil {
		t.Fatalf("ReadFull() error = %v", err)
	}
	if string(response) != expected {
		t.Fatalf("response = %q, want %q", response, expected)
	}
}
