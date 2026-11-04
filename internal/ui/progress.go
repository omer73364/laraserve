package ui

import (
	"fmt"
	"laraserve/internal/activity"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

// Progress is a synchronous writer: it creates no background timers or output
// races with Composer, prompts, or errors. Pipes receive occasional plain lines.
type Progress struct {
	r             renderer
	label         string
	total, n      int64
	started, last time.Time
	previous      int
}

func NewProgress(label string, total int64) *Progress {
	now := time.Now()
	return &Progress{r: terminal(os.Stdout), label: label, total: total, started: now, last: now}
}

func (p *Progress) Write(data []byte) (int, error) {
	p.n += int64(len(data))
	interval := 5 * time.Second
	if p.r.interactive {
		interval = 150 * time.Millisecond
	}
	if time.Since(p.last) >= interval {
		p.draw(time.Now())
		p.last = time.Now()
	}
	return len(data), nil
}

func (p *Progress) draw(now time.Time) {
	if p.total > 0 {
		activity.Detail(fmt.Sprintf("%s · %.0f%% (%s / %s)", p.label, float64(p.n)*100/float64(p.total), Bytes(p.n), Bytes(p.total)))
	} else {
		activity.Detail(fmt.Sprintf("%s · %s downloaded", p.label, Bytes(p.n)))
	}
	elapsed := now.Sub(p.started).Seconds()
	speed := float64(p.n)
	if elapsed > 0 {
		speed /= elapsed
	}
	detail := Bytes(p.n)
	if p.total > 0 {
		ratio := float64(p.n) / float64(p.total)
		if ratio > 1 {
			ratio = 1
		}
		if ratio < 0 {
			ratio = 0
		}
		detail = fmt.Sprintf("%3.0f%%  %s / %s", ratio*100, Bytes(p.n), Bytes(p.total))
		if p.r.interactive && p.r.width >= 70 {
			filled := int(ratio * 16)
			detail = "[" + strings.Repeat("=", filled) + strings.Repeat("-", 16-filled) + "] " + detail
		}
		if p.r.width >= 70 && speed > 0 && p.total > p.n {
			eta := time.Duration(float64(p.total-p.n)/speed) * time.Second
			if eta >= time.Second {
				detail += "  ETA " + eta.String()
			}
		}
	}
	if p.r.width < 50 {
		if p.total > 0 {
			detail = fmt.Sprintf("%3.0f%%  %s", minRatio(p.n, p.total)*100, Bytes(p.n))
		} else {
			detail = Bytes(p.n)
		}
	} else {
		detail += "  " + Bytes(int64(speed)) + "/s"
	}
	if !p.r.interactive {
		p.r.message("info", "%s: %s", p.label, detail)
		return
	}
	// Keep the counters visible in narrow terminals; the initial download message
	// already carries the complete tool name.
	label := p.label
	budget := p.r.width - utf8.RuneCountInString(detail) - 6
	if budget < 0 {
		budget = 0
	}
	if utf8.RuneCountInString(label) > budget {
		label = string([]rune(label)[:budget])
	}
	line := "  " + label + "  " + detail
	visible := utf8.RuneCountInString(line)
	padding := 0
	if p.previous > visible {
		padding = p.previous - visible
	}
	fmt.Fprintf(p.r.out, "\r%s%s", p.r.paint(cyan, line), strings.Repeat(" ", padding))
	p.previous = visible
}

// Finish clears a live progress line before the final result or an error. It is
// safe to defer, including for downloads that fail a checksum check.
func (p *Progress) Finish() {
	if p.r.interactive && p.previous > 0 {
		fmt.Fprintf(p.r.out, "\r%s\r", strings.Repeat(" ", p.previous))
		p.previous = 0
	}
}

func minRatio(n, total int64) float64 {
	ratio := float64(n) / float64(total)
	if ratio > 1 {
		return 1
	}
	if ratio < 0 {
		return 0
	}
	return ratio
}
