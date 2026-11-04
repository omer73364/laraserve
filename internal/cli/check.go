package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"laraserve/internal/projectcheck"
	"laraserve/internal/ui"
)

func cmdCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	jsonOutput := fs.Bool("json", false, "print the offline project check as JSON")
	if err := parse(fs, args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return fmt.Errorf("usage: laraserve check [path] [--json]")
	}
	target := "."
	if fs.NArg() == 1 {
		target = fs.Arg(0)
	}
	root, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	home := os.Getenv("LARASERVE_HOME")
	if home == "" {
		base, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		home = filepath.Join(base, ".laraserve")
	}
	report := projectcheck.Inspect(root, filepath.Join(home, "runtimes"))
	if *jsonOutput {
		if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
			return err
		}
	} else {
		ui.Section("Project check · no files changed")
		for _, check := range report.Checks {
			text := check.Title + ": " + check.Detail
			switch check.Status {
			case "error", "warning":
				ui.Warn("%s", text)
			case "ok":
				ui.OK("%s", text)
			default:
				ui.Info("%s", text)
			}
		}
		ui.Info("%d issues need fixing · %d warnings. Startup also verifies downloads, ports and service readiness.", report.Errors, report.Warnings)
	}
	if report.Errors > 0 {
		return fmt.Errorf("project check found %d issue(s) requiring attention", report.Errors)
	}
	return nil
}
