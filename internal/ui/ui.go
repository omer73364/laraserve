// Package ui renders terminal output and conservative confirmation prompts.
package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// Yes answers every confirmation prompt with yes (--yes).
var Yes bool

const (
	accent = "\x1b[92m"
	cyan   = "\x1b[36m"
	yellow = "\x1b[33m"
	red    = "\x1b[31m"
	muted  = "\x1b[90m"
	bold   = "\x1b[1m"
	reset  = "\x1b[0m"
)

type renderer struct {
	out                io.Writer
	color, interactive bool
	width              int
}

func colorEnabled(tty bool) bool {
	if _, set := os.LookupEnv("NO_COLOR"); set {
		return false
	}
	if os.Getenv("TERM") == "dumb" || os.Getenv("CLICOLOR") == "0" {
		return false
	}
	for _, name := range []string{"FORCE_COLOR", "CLICOLOR_FORCE"} {
		if value := os.Getenv(name); value != "" && value != "0" {
			return true
		}
	}
	return tty
}

func terminal(file *os.File) renderer {
	tty := term.IsTerminal(int(file.Fd()))
	width := 80
	if w, _, err := term.GetSize(int(file.Fd())); err == nil && w > 0 {
		width = w
	}
	return renderer{out: file, color: colorEnabled(tty), interactive: tty && os.Getenv("TERM") != "dumb", width: width}
}

func (r renderer) paint(style, text string) string {
	if !r.color {
		return text
	}
	return style + text + reset
}

func (r renderer) message(kind, format string, args ...any) {
	marks := map[string]string{"ok": "[ok]", "info": "[info]", "warn": "[warn]", "error": "[error]"}
	styles := map[string]string{"ok": accent, "info": cyan, "warn": yellow, "error": red}
	if r.interactive {
		marks = map[string]string{"ok": "✓", "info": "›", "warn": "!", "error": "✕"}
	}
	lines := strings.Split(fmt.Sprintf(format, args...), "\n")
	fmt.Fprintf(r.out, "  %s %s\n", r.paint(styles[kind], marks[kind]), lines[0])
	for _, line := range lines[1:] {
		fmt.Fprintf(r.out, "      %s\n", line)
	}
}

func OK(format string, args ...any)   { terminal(os.Stdout).message("ok", format, args...) }
func Info(format string, args ...any) { terminal(os.Stdout).message("info", format, args...) }
func Warn(format string, args ...any) { terminal(os.Stdout).message("warn", format, args...) }
func Err(format string, args ...any)  { terminal(os.Stderr).message("error", format, args...) }

// Confirm defaults to yes only when the user has an interactive input terminal.
// Pipes and redirected input never accept host changes without --yes.
func Confirm(question string) bool {
	r := terminal(os.Stdout)
	prompt := r.paint(yellow, "?") + " " + question
	if Yes {
		fmt.Fprintf(r.out, "\n  %s [Y/n] y (--yes)\n", prompt)
		return true
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintf(r.out, "\n  %s\n      Skipped: non-interactive input. Use --yes to accept.\n", prompt)
		return false
	}
	fmt.Fprintf(r.out, "\n  %s %s ", prompt, r.paint(muted, "[Y/n]"))
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return false
	}
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "" || line == "y" || line == "yes"
}
