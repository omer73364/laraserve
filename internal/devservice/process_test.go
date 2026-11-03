package devservice

import (
	"context"
	"laraserve/internal/process"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestServiceHelper(t *testing.T) {
	mode := os.Getenv("LARASERVE_SERVICE_HELPER")
	if mode == "" {
		return
	}
	if mode == "fail" {
		os.Exit(7)
	}
	listener, err := net.Listen("tcp4", mode)
	if err != nil {
		os.Exit(8)
	}
	defer listener.Close()
	for {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		connection.Close()
	}
}
func TestServiceReadinessAndCancellation(t *testing.T) {
	if err := process.InitOwnership(); err != nil {
		t.Fatal(err)
	}
	listener, err := Reserve()
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	executable, _ := os.Executable()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	command := Command{Label: "test", Root: t.TempDir(), Log: filepath.Join(t.TempDir(), "service.log"), Address: address, Executable: executable, Args: []string{"-test.run=^TestServiceHelper$"}, Env: append(os.Environ(), "LARASERVE_SERVICE_HELPER="+address)}
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- command.Run(ctx, func() { close(ready) }) }()
	select {
	case <-ready:
	case err := <-done:
		t.Fatal(err)
	case <-time.After(10 * time.Second):
		t.Fatal("service never ready")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("service survived cancellation")
	}
	connection, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
	if err == nil {
		connection.Close()
		t.Fatal("listener leaked")
	}
}
func TestServiceEarlyFailure(t *testing.T) {
	executable, _ := os.Executable()
	command := Command{Label: "test", Root: t.TempDir(), Log: filepath.Join(t.TempDir(), "service.log"), Executable: executable, Args: []string{"-test.run=^TestServiceHelper$"}, Env: append(os.Environ(), "LARASERVE_SERVICE_HELPER=fail")}
	wasReady := false
	err := command.Run(context.Background(), func() { wasReady = true })
	if err == nil || !strings.Contains(err.Error(), "exit status 7") || wasReady {
		t.Fatal(err, wasReady)
	}
}
