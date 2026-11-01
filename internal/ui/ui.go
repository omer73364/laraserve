// Package ui holds tiny helpers for terminal output and prompts.
package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Yes answers every confirmation prompt with "yes" (--yes).
var Yes bool

func OK(format string, a ...any)   { fmt.Printf("✔ "+format+"\n", a...) }
func Info(format string, a ...any) { fmt.Printf("  "+format+"\n", a...) }
func Warn(format string, a ...any) { fmt.Printf("! "+format+"\n", a...) }
func Err(format string, a ...any)  { fmt.Fprintf(os.Stderr, "✖ "+format+"\n", a...) }

func isTTY() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// Confirm asks a yes/no question (default yes). Without a terminal and
// without --yes it answers no, so nothing on the host changes silently.
func Confirm(question string) bool {
	if Yes {
		fmt.Printf("? %s [Y/n] y (--yes)\n", question)
		return true
	}
	if !isTTY() {
		fmt.Printf("? %s -> skipped (non-interactive, pass --yes to accept)\n", question)
		return false
	}
	fmt.Printf("? %s [Y/n] ", question)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "" || line == "y" || line == "yes"
}
