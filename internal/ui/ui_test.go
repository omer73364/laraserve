package ui

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestColorPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, term, force, cli string
		tty, noColor, want     bool
	}{
		{name: "terminal", tty: true, want: true},
		{name: "pipe", want: false},
		{name: "forced pipe", force: "1", want: true},
		{name: "NO_COLOR wins over forced", force: "1", tty: true, noColor: true},
		{name: "dumb terminal", term: "dumb", tty: true},
		{name: "CLICOLOR disabled", tty: true, cli: "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, name := range []string{"NO_COLOR", "FORCE_COLOR", "CLICOLOR_FORCE", "CLICOLOR", "TERM"} {
				t.Setenv(name, "")
			}
			if !tc.noColor {
				os.Unsetenv("NO_COLOR")
			}
			t.Setenv("TERM", tc.term)
			t.Setenv("FORCE_COLOR", tc.force)
			t.Setenv("CLICOLOR", tc.cli)
			if got := colorEnabled(tc.tty); got != tc.want {
				t.Fatalf("color=%v want %v", got, tc.want)
			}
		})
	}
}

func TestPlainMessagesAndMultilineErrors(t *testing.T) {
	var out bytes.Buffer
	r := renderer{out: &out, width: 80}
	r.message("error", "migration failed\ninspect %s", "/tmp/project with spaces")
	r.section("Application ready")
	got := out.String()
	if strings.Contains(got, "\x1b") || strings.Contains(got, "\r") {
		t.Fatal("escape sequences in plain output")
	}
	if !strings.Contains(got, "[error] migration failed\n      inspect /tmp/project with spaces") {
		t.Fatal(got)
	}
}

func TestSummaryPreservesCompletePathsAndSignInURLs(t *testing.T) {
	url := "https://localhost:8444/laraserve-login.php?token=" + strings.Repeat("a", 64)
	path := "/home/developer/projects/a-long-project-name with spaces"
	for _, width := range []int{35, 80} {
		var out bytes.Buffer
		r := renderer{out: &out, width: width}
		r.fields([]Field{{Label: "Project", Value: path}, {Label: "phpMyAdmin", Value: url}})
		if !strings.Contains(out.String(), url) || !strings.Contains(out.String(), path) {
			t.Fatal("summary truncated data", out.String())
		}
	}
}

func TestDownloadProgressCountersAndCleanup(t *testing.T) {
	var out bytes.Buffer
	now := time.Unix(100, 0)
	p := Progress{r: renderer{out: &out, interactive: true, width: 100}, label: "PHP", total: 1024 * 1024, n: 256 * 1024, started: now.Add(-10 * time.Second)}
	p.draw(now)
	text := out.String()
	for _, want := range []string{"25%", "256.0 KiB / 1.0 MiB", "25.6 KiB/s", "ETA 30s"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	p.Finish()
	if p.previous != 0 || !strings.HasSuffix(out.String(), "\r") {
		t.Fatal("progress was not cleared")
	}
	before := out.Len()
	p.Finish()
	if out.Len() != before {
		t.Fatal("cleanup is not idempotent")
	}
}

func TestProgressFitsNarrowTerminalAndHandlesUnknownSize(t *testing.T) {
	for _, width := range []int{20, 35, 50, 80} {
		for _, total := range []int64{-1, 1024 * 1024} {
			var out bytes.Buffer
			now := time.Unix(100, 0)
			p := Progress{r: renderer{out: &out, interactive: true, width: width}, label: "PHP 7.4 support library", total: total, n: 32768, started: now.Add(-time.Second)}
			p.draw(now)
			line := strings.TrimPrefix(out.String(), "\r")
			if utf8.RuneCountInString(line) > width {
				t.Fatalf("progress exceeds width %d: %s", width, line)
			}
			if strings.Contains(line, "NaN") || strings.Contains(line, "Inf") {
				t.Fatal(line)
			}
		}
	}
}

func TestProgressPipeHasNoCursorControl(t *testing.T) {
	var out bytes.Buffer
	now := time.Now()
	p := Progress{r: renderer{out: &out, width: 80}, label: "MySQL", n: 2048, total: 4096, started: now.Add(-time.Second)}
	p.draw(now)
	p.Finish()
	if strings.ContainsAny(out.String(), "\x1b\r") {
		t.Fatal("cursor control leaked into redirected output")
	}
	if !strings.Contains(out.String(), "50%") {
		t.Fatal(out.String())
	}
}

func TestHumanReadableByteSizes(t *testing.T) {
	for n, want := range map[int64]string{0: "0 B", 999: "999 B", 1024: "1.0 KiB", 7184: "7.0 KiB", 1048576: "1.0 MiB"} {
		if got := Bytes(n); got != want {
			t.Fatalf("%d: got %s want %s", n, got, want)
		}
	}
}

func TestNonInteractiveInputDoesNotConfirm(t *testing.T) {
	oldInput, oldYes := os.Stdin, Yes
	t.Cleanup(func() { os.Stdin = oldInput; Yes = oldYes })
	input, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	os.Stdin = input
	Yes = false
	if Confirm("Test prompt") {
		t.Fatal("non-interactive input accepted host changes")
	}
}
