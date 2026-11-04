package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	goruntime "runtime"
	"time"

	"laraserve/internal/manifest"
	"laraserve/internal/paths"
	"laraserve/internal/runtime"
	"laraserve/internal/ui"
)

type diagnosticCheck struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}
type supportReport struct {
	Schema         int               `json:"schema"`
	Version        string            `json:"laraserve_version"`
	Platform       string            `json:"platform"`
	Created        string            `json:"created_utc"`
	Projects       int               `json:"project_count"`
	CachedRuntimes int               `json:"cached_tool_count"`
	Checks         []diagnosticCheck `json:"checks"`
}

// Reports use an allowlist of controlled messages and counts. Never copy raw
// errors, environment variables, project paths, domains, configs, or logs.
func collectSupportReport(home *paths.Home) supportReport {
	r := supportReport{Schema: 1, Version: runtime.Version, Platform: goruntime.GOOS + "/" + goruntime.GOARCH, Created: time.Now().UTC().Format(time.RFC3339)}
	add := func(id, status, message string) { r.Checks = append(r.Checks, diagnosticCheck{id, status, message}) }
	m, err := manifest.Load(home.Manifest())
	if err != nil {
		add("manifest", "warning", "Local state could not be read; preserve it and inspect file access or JSON formatting.")
	} else {
		r.Projects = len(m.Projects)
		add("manifest", "ok", "Local state is readable.")
		if m.CA != nil {
			add("trust_record", "info", "Prior certificate trust approval is recorded; normal HTTPS startup checks browser imports.")
		} else {
			add("trust_record", "info", "Certificate trust is not recorded; normal HTTPS startup offers setup.")
		}
	}
	entries, err := os.ReadDir(home.Runtimes())
	if err != nil && !os.IsNotExist(err) {
		add("runtime_cache", "warning", "The runtime cache could not be read; check filesystem access.")
	} else {
		for _, entry := range entries {
			if entry.IsDir() {
				if st, err := os.Stat(filepath.Join(home.Runtimes(), entry.Name(), ".ready")); err == nil && st.Mode().IsRegular() {
					r.CachedRuntimes++
				}
			}
		}
		if r.CachedRuntimes == 0 {
			add("runtime_cache", "info", "No completed tool downloads are cached; first startup needs internet access.")
		} else {
			add("runtime_cache", "ok", "Completed tool downloads are cached; startup validates and repairs selected runtimes as needed.")
		}
	}
	for _, port := range []string{"443", "8443", "8080"} {
		listener, err := net.Listen("tcp4", "127.0.0.1:"+port)
		if err != nil {
			add("port_"+port, "info", "Port "+port+" is unavailable; use --port to choose another port.")
		} else {
			listener.Close()
			add("port_"+port, "ok", "Port "+port+" is available.")
		}
	}
	return r
}

func cmdDoctor(args ...string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	output := fs.String("report", "", "write a private support report with no paths, environment variables, credentials, or logs")
	if err := parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: laraserve doctor [--report FILE]")
	}
	home, err := paths.New()
	if err != nil {
		return err
	}
	report := collectSupportReport(home)
	if *output != "" {
		b, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		f, err := os.OpenFile(*output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("cannot create support report (choose a new filename if it exists): %w", err)
		}
		_, writeErr := f.Write(append(b, '\n'))
		closeErr := f.Close()
		if writeErr != nil {
			os.Remove(*output)
			return writeErr
		}
		if closeErr != nil {
			os.Remove(*output)
			return closeErr
		}
		ui.OK("Support report saved to %s", *output)
		return nil
	}
	ui.Fields("Environment", ui.Field{Label: "Platform", Value: report.Platform}, ui.Field{Label: "Home", Value: home.Root}, ui.Field{Label: "Projects", Value: fmt.Sprint(report.Projects)}, ui.Field{Label: "Cached tools", Value: fmt.Sprint(report.CachedRuntimes)})
	ui.Section("Health checks")
	for _, check := range report.Checks {
		switch check.Status {
		case "warning":
			ui.Warn("%s", check.Message)
		case "ok":
			ui.OK("%s", check.Message)
		default:
			ui.Info("%s", check.Message)
		}
	}
	ui.Info("For a private support report: laraserve doctor --report report.json")
	return nil
}
