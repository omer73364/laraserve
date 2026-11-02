package cli

import (
	"errors"
	"net"
	"syscall"
	"testing"

	"laraserve/internal/runtime"
)

func TestBusyHTTPSPortFallsBackWithoutCapabilityPrompt(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:443")
	if err == nil {
		defer listener.Close()
	} else if !errors.Is(err, syscall.EADDRINUSE) {
		t.Skipf("cannot reserve or inspect port 443: %v", err)
	}
	selected, err := choosePort(&runtime.Runtime{PHP: "/nonexistent-server"}, "127.0.0.1", 0, true, true)
	if err != nil || selected < 8443 {
		t.Fatalf("busy port 443 did not fall back: %d %v", selected, err)
	}
	if _, err := choosePort(&runtime.Runtime{}, "127.0.0.1", 443, true, true); err == nil {
		t.Fatal("explicit occupied port was accepted")
	}
}
