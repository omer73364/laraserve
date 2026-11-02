package serve

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func checkPort(host string, port int) error {
	listener, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err == nil {
		listener.Close()
		return nil
	}
	// The server may have a low-port capability which the client lacks.
	if errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) {
		return nil
	}
	if errors.Is(err, syscall.EADDRINUSE) {
		return fmt.Errorf("port %d is already in use; choose another port with --port (for example --port 8443)", port)
	}
	return fmt.Errorf("cannot bind %s:%d: %w", host, port, err)
}

func waitReady(parent context.Context, c Config) error {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	host := c.Host
	if host == "0.0.0.0" {
		host = "127.0.0.1"
	} else if host == "::" {
		host = "::1"
	}
	address := net.JoinHostPort(host, strconv.Itoa(c.Port))
	transport := &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 300 * time.Millisecond}).DialContext(ctx, network, address)
	}}
	defer transport.CloseIdleConnections()
	if c.HTTPS {
		pem, err := os.ReadFile(c.Cert)
		if err != nil {
			return err
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(pem) {
			return fmt.Errorf("invalid readiness TLS certificate")
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: roots, ServerName: c.Domain, MinVersion: tls.VersionTLS12}
	}
	client := &http.Client{Transport: transport, Timeout: 500 * time.Millisecond, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL()+"/.__laraserve_ready_"+c.readinessToken, nil)
		if err != nil {
			return err
		}
		response, err := client.Do(req)
		if err == nil {
			data, readErr := io.ReadAll(io.LimitReader(response.Body, 256))
			response.Body.Close()
			if readErr == nil && response.StatusCode == http.StatusOK && string(data) == c.readinessToken {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func serverFailure(c Config, err error, logPath string) error {
	logs := tail(logPath, 30)
	if strings.Contains(logs, "address already in use") {
		return fmt.Errorf("server could not bind its listening port (app port %d is configured); choose a free --port such as 8443. Full log: %s", c.Port, logPath)
	}
	var fatal string
	for _, line := range strings.Split(logs, "\n") {
		var event struct{ Level, Msg, Error string }
		if json.Unmarshal([]byte(line), &event) == nil && event.Level == "error" {
			fatal = event.Msg
			if event.Error != "" {
				fatal += ": " + event.Error
			}
		}
	}
	if fatal != "" {
		return fmt.Errorf("server exited: %v\n%s\nFull log: %s", err, fatal, logPath)
	}
	return fmt.Errorf("server exited: %v\n%s\nFull log: %s", err, logs, logPath)
}
