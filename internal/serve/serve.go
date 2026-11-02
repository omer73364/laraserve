// Package serve generates the Caddyfile and supervises FrankenPHP.
package serve

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Config struct {
	readinessToken string
	NativePHP      string
	BackendPort    int
	Admin          *AdminConfig
	Context        context.Context // optional parent lifetime (managed database)
	PHP            string          // frankenphp binary
	Root           string          // project root (contains public/)
	StateDir       string
	Env            []string
	AppEnv         map[string]string // also injected via php_server `env`
	Domain         string
	Host           string // bind address
	Port           int
	HTTPS          bool
	Cert           string
	Key            string
	LAN            bool // accept any Host header (for --host 0.0.0.0)
	Verbose        bool
}

// AdminConfig is always bound to IPv4 loopback, independently of the app host.
type AdminConfig struct {
	BackendPort int
	Root        string
	Port        int
}

// URL is the address a user opens in the browser.
func (c Config) URL() string {
	scheme, def := "http", 80
	if c.HTTPS {
		scheme, def = "https", 443
	}
	if c.Port == def {
		return fmt.Sprintf("%s://%s", scheme, c.Domain)
	}
	return fmt.Sprintf("%s://%s:%d", scheme, c.Domain, c.Port)
}

func q(s string) string { return strconv.Quote(filepath.ToSlash(s)) }

// Caddyfile renders the server configuration.
func Caddyfile(c Config) string {
	scheme := "http"
	if c.HTTPS {
		scheme = "https"
	}
	var addrs []string
	if c.LAN {
		addrs = []string{fmt.Sprintf("%s://:%d", scheme, c.Port)}
	} else {
		addrs = []string{fmt.Sprintf("%s://%s:%d", scheme, c.Domain, c.Port)}
		if c.Domain != "localhost" {
			addrs = append(addrs, fmt.Sprintf("%s://localhost:%d", scheme, c.Port))
		}
	}
	var b strings.Builder
	b.WriteString("{\n\tadmin off\n\tauto_https off\n\tskip_install_trust\n\tpki {\n\t\tca local {\n\t\t}\n\t}\n}\n\n")
	b.WriteString(strings.Join(addrs, ", ") + " {\n")
	fmt.Fprintf(&b, "\tbind %s\n", c.Host)
	if c.HTTPS {
		fmt.Fprintf(&b, "\ttls %s %s\n", q(c.Cert), q(c.Key))
	}
	if c.readinessToken != "" {
		fmt.Fprintf(&b, "\trespond /.__laraserve_ready_%s %q 200\n", c.readinessToken, c.readinessToken)
	}
	fmt.Fprintf(&b, "\troot * %s\n", q(filepath.Join(c.Root, "public")))
	if c.BackendPort != 0 {
		b.WriteString(nativeProxy(c.BackendPort))
	} else {
		b.WriteString("\tphp_server")
		if len(c.AppEnv) > 0 {
			b.WriteString(" {\n")
			keys := make([]string, 0, len(c.AppEnv))
			for k := range c.AppEnv {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(&b, "\t\tenv %s %s\n", k, strconv.Quote(c.AppEnv[k]))
			}
			b.WriteString("\t}")
		}
	}
	b.WriteString("\n}\n")
	if c.Admin != nil {
		fmt.Fprintf(&b, "\n%s://localhost:%d {\n\tbind 127.0.0.1\n", scheme, c.Admin.Port)
		if c.HTTPS {
			fmt.Fprintf(&b, "\ttls %s %s\n", q(c.Cert), q(c.Key))
		}
		fmt.Fprintf(&b, "\troot * %s\n", q(c.Admin.Root))
		b.WriteString("\t@privateFiles path /config.inc.php /libraries/* /templates/* /vendor/* /setup/* /examples/* /sql/* /.*\n")
		b.WriteString("\trespond @privateFiles 404\n")
		if c.Admin.BackendPort != 0 {
			b.WriteString(nativeProxy(c.Admin.BackendPort))
		} else {
			b.WriteString("\tphp_server\n")
		}
		b.WriteString("}\n")
	}
	return b.String()
}

// FreePort returns start if free; otherwise (when !strict) the next free port.
func FreePort(host string, start int, strict bool) (int, error) {
	for p := start; p < start+20; p++ {
		l, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(p)))
		if err == nil {
			l.Close()
			return p, nil
		}
		if strict {
			return 0, fmt.Errorf("port %d is not available: %w", p, err)
		}
	}
	return 0, fmt.Errorf("no free port found from %d", start)
}

// CanBind reports whether the current user may listen on host:port.
func CanBind(host string, port int) bool {
	l, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return false
	}
	l.Close()
	return true
}

// Run starts FrankenPHP and blocks until it exits or Ctrl+C is pressed.
// onReady is called only after this launch answers its private readiness probe.
func Run(c Config, onReady func()) error {
	if err := checkPort(c.Host, c.Port); err != nil {
		return err
	}
	if c.Admin != nil {
		if err := checkPort("127.0.0.1", c.Admin.Port); err != nil {
			return err
		}
	}
	token := make([]byte, 24)
	if _, err := rand.Read(token); err != nil {
		return err
	}
	c.readinessToken = hex.EncodeToString(token)
	if err := os.MkdirAll(filepath.Join(c.StateDir, "logs"), 0o755); err != nil {
		return err
	}
	logPath := filepath.Join(c.StateDir, "logs", "server.log")
	logf, err := os.Create(logPath)
	if err != nil {
		return err
	}
	defer logf.Close()

	parent := c.Context
	if parent == nil {
		parent = context.Background()
	}
	signalCtx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithCancel(signalCtx)
	defer cancel()
	backendFailures := make(chan error, 2)
	nativeStop := func() {
		select {
		case backendFailures <- fmt.Errorf("selected PHP backend stopped unexpectedly"):
		default:
		}
		cancel()
	}
	var backendCleanup []func()
	defer func() {
		cancel()
		for _, cleanup := range backendCleanup {
			cleanup()
		}
	}()
	if c.NativePHP != "" {
		p, cleanup, err := startNative(ctx, c, filepath.Join(c.Root, "public"), logf, nativeStop)
		if err != nil {
			return err
		}
		c.BackendPort = p
		backendCleanup = append(backendCleanup, cleanup)
		if c.Admin != nil {
			p, cleanup, err = startNative(ctx, c, c.Admin.Root, logf, nativeStop)
			if err != nil {
				return err
			}
			c.Admin.BackendPort = p
			backendCleanup = append(backendCleanup, cleanup)
		}
	}
	cf := filepath.Join(c.StateDir, "Caddyfile")
	if err := os.WriteFile(cf, []byte(Caddyfile(c)), 0o600); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, c.PHP, "run", "--config", cf, "--adapter", "caddyfile")
	cmd.Dir = c.Root
	cmd.Env = c.Env
	if c.Verbose {
		cmd.Stdout, cmd.Stderr = io.MultiWriter(logf, os.Stdout), io.MultiWriter(logf, os.Stderr)
	} else {
		cmd.Stdout, cmd.Stderr = logf, logf
	}
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 5 * time.Second
	if err := cmd.Start(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	ready := make(chan error, 1)
	go func() { ready <- waitReady(ctx, c) }()

	for {
		select {
		case probeErr := <-ready:
			if probeErr != nil {
				cancel()
				<-done
				select {
				case failure := <-backendFailures:
					return fmt.Errorf("%w; see %s", failure, logPath)
				default:
				}
				if signalCtx.Err() != nil {
					return nil
				}
				return fmt.Errorf("server did not become ready: %w; see %s", probeErr, logPath)
			}
			// A failed child can race with its last response. Do not announce it.
			select {
			case exitErr := <-done:
				if signalCtx.Err() != nil {
					return nil
				}
				return serverFailure(c, exitErr, logPath)
			default:
			}
			if onReady != nil {
				onReady()
			}
			ready = nil
		case err := <-done:
			select {
			case failure := <-backendFailures:
				return fmt.Errorf("%w\n%s", failure, tail(logPath, 15))
			default:
			}
			if ctx.Err() != nil {
				return nil // stopped by Ctrl+C
			}
			return serverFailure(c, err, logPath)
		}
	}
}

func tail(path string, n int) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		lines = append(lines, sc.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	return strings.Join(lines, "\n")
}
