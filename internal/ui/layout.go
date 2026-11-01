package ui

import (
	"fmt"
	"os"
	"strings"
)

// Field keeps labels aligned while preserving complete paths and sign-in URLs.
type Field struct{ Label, Value string }

func Header(version, action string) { terminal(os.Stdout).header(version, action) }
func Section(title string)          { terminal(os.Stdout).section(title) }
func Fields(title string, fields ...Field) {
	r := terminal(os.Stdout)
	r.section(title)
	r.fields(fields)
}
func Ready(fields ...Field) {
	r := terminal(os.Stdout)
	r.section("Application ready")
	r.fields(fields)
	fmt.Fprintf(r.out, "\n  %s\n\n", r.paint(muted, "Press Ctrl+C to stop"))
}

func (r renderer) header(version, action string) {
	fmt.Fprintf(r.out, "\n  %s %s\n  %s\n\n", r.paint(bold+accent, "LARASERVE"), r.paint(muted, "v"+version), r.paint(muted, action))
}

func (r renderer) section(title string) {
	width := r.width - 4
	if width > 72 {
		width = 72
	}
	if width < 8 {
		width = 8
	}
	rule := "─"
	if !r.interactive {
		rule = "-"
	}
	fmt.Fprintf(r.out, "\n  %s\n  %s\n", r.paint(bold+accent, title), r.paint(muted, strings.Repeat(rule, width)))
}

func (r renderer) fields(fields []Field) {
	labelWidth := 0
	for _, field := range fields {
		if len(field.Label) > labelWidth {
			labelWidth = len(field.Label)
		}
	}
	for _, field := range fields {
		style := ""
		if strings.HasPrefix(field.Value, "http://") || strings.HasPrefix(field.Value, "https://") {
			style = bold + cyan
		}
		value := field.Value
		if style != "" {
			value = r.paint(style, value)
		}
		if r.width < 50 {
			fmt.Fprintf(r.out, "  %s\n    %s\n", r.paint(muted, field.Label), value)
		} else {
			fmt.Fprintf(r.out, "  %s  %s\n", r.paint(muted, fmt.Sprintf("%-*s", labelWidth, field.Label)), value)
		}
	}
}

func Bytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n) / 1024
	for _, unit := range []string{"KiB", "MiB", "GiB", "TiB"} {
		if value < 1024 || unit == "TiB" {
			return fmt.Sprintf("%.1f %s", value, unit)
		}
		value /= 1024
	}
	return ""
}
