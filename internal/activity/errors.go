package activity

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
)

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// Failure returns the actual bounded command error, not a pointer to a log file.
func Failure(file, fallback string) string {
	f, e := os.Open(file)
	if e != nil {
		return fallback
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return fallback
	}
	if st.Size() > 64<<10 {
		_, _ = f.Seek(st.Size()-(64<<10), io.SeekStart)
	}
	b, _ := io.ReadAll(io.LimitReader(f, 64<<10))
	lines := strings.Split(ansi.ReplaceAllString(string(b), ""), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.Contains(line, "[error]") {
			out := strings.TrimSpace(strings.SplitN(line, "[error]", 2)[1])
			for j := i + 1; j < len(lines) && j < i+7; j++ {
				next := strings.TrimSpace(lines[j])
				if next != "" {
					out += "\n" + next
				}
			}
			return bounded(out)
		}
	}
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "npm error") || strings.HasPrefix(line, "Error:") {
			return bounded(line)
		}
	}
	return fallback
}
func bounded(s string) string {
	if len(s) > 3000 {
		return s[:3000]
	}
	return s
}

// Output retains a bounded tail while forwarding output to the project log.
type Output struct {
	mu          sync.Mutex
	Destination io.Writer
	tail        []byte
}

func (w *Output) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.tail = append(w.tail, b...)
	if len(w.tail) > 8192 {
		w.tail = append([]byte(nil), w.tail[len(w.tail)-8192:]...)
	}
	if w.Destination != nil {
		return w.Destination.Write(b)
	}
	return len(b), nil
}
func (w *Output) Error(err error) error {
	if err == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	text := strings.TrimSpace(ansi.ReplaceAllString(string(w.tail), ""))
	lines := strings.Split(text, "\n")
	if len(lines) > 18 {
		lines = lines[len(lines)-18:]
	}
	return fmt.Errorf("%w\n%s", err, bounded(strings.Join(lines, "\n")))
}
