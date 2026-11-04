package cli

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"laraserve/internal/appsettings"
	"laraserve/internal/dashboard"
	"laraserve/internal/paths"
	"laraserve/internal/runtime"
	"laraserve/internal/ui"
)

func cmdDashboard(args []string) error {
	fs := flag.NewFlagSet("dashboard", flag.ContinueOnError)
	open := fs.Bool("open", false, "open the private dashboard in the default browser")
	port := fs.Int("port", 0, "local dashboard port (0 selects a free port)")
	if err := parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 0 || *port < 0 || *port > 65535 {
		return fmt.Errorf("usage: laraserve dashboard [--port N]")
	}
	home, err := paths.New()
	if err != nil {
		return err
	}
	if _, err = projectStore(home); err != nil {
		return err
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(*port)))
	if err != nil {
		return err
	}
	defer listener.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	handler, err := dashboard.New(ctx, home.Root, listener.Addr().String())
	if err != nil {
		return err
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	ui.Fields("LaraServe dashboard", ui.Field{Label: "Open", Value: handler.URL()}, ui.Field{Label: "Access", Value: "Private to this computer · keep this link private"})
	go runStartupProjects(ctx, home.Root)
	if doc, e := appsettings.Read(home.Root); e == nil && doc.Config.CheckUpdates {
		go func() { _, _ = appsettings.Check(ctx, home.Root, runtime.Version, false) }()
	}
	if *open {
		if e := openDashboardURL(handler.URL()); e != nil {
			ui.Warn("%v", e)
		}
	}
	ui.Info("Press Ctrl+C to close the dashboard; your projects keep running")
	select {
	case <-handler.Done():
	case <-ctx.Done():
	case err = <-done:
		if err != http.ErrServerClosed {
			return err
		}
	}
	shutdown, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	if err = server.Shutdown(shutdown); err != nil {
		_ = server.Close()
	}
	return nil
}
