// Package hosts manages a clearly marked block in the OS hosts file.
package hosts

import (
	"errors"
	"fmt"
	"laraserve/internal/activity"
	"laraserve/internal/elevate"
	"net"
	"os"
	goruntime "runtime"
	"sort"
	"strings"
)

const (
	begin = "# BEGIN laraserve"
	end   = "# END laraserve"
)

func Path() string {
	if goruntime.GOOS == "windows" {
		return os.Getenv("SystemRoot") + `\System32\drivers\etc\hosts`
	}
	return "/etc/hosts"
}

// IsLoopbackName is true for names that resolve to 127.0.0.1 without any setup.
func IsLoopbackName(d string) bool {
	return d == "localhost" || strings.HasSuffix(d, ".localhost")
}

// Resolves reports whether the OS already resolves d to a loopback address.
func Resolves(d string) bool {
	ips, err := net.LookupIP(d)
	if err != nil {
		return false
	}
	for _, ip := range ips {
		if ip.IsLoopback() {
			return true
		}
	}
	return false
}

// Line is the entry a user can add manually.
func Line(d string) string { return "127.0.0.1 " + d }

// Add ensures d is present in the laraserve block.
func Add(d string) error {
	return update(func(set map[string]bool) { set[d] = true })
}

// Remove deletes d from the block (and the block itself when empty).
func Remove(d string) error {
	return update(func(set map[string]bool) { delete(set, d) })
}

func update(mut func(map[string]bool)) error {
	cur, err := os.ReadFile(Path())
	if err != nil {
		return err
	}
	next, changed := Rewrite(string(cur), mut)
	if !changed {
		return nil
	}
	return write(next)
}

// Rewrite applies mut to the set of domains in the laraserve block.
func Rewrite(content string, mut func(map[string]bool)) (string, bool) {
	var outside []string
	set := map[string]bool{}
	in := false
	for _, ln := range strings.Split(strings.TrimRight(content, "\n"), "\n") {
		switch {
		case strings.TrimSpace(ln) == begin:
			in = true
		case strings.TrimSpace(ln) == end:
			in = false
		case in:
			if f := strings.Fields(ln); len(f) >= 2 {
				set[f[1]] = true
			}
		default:
			outside = append(outside, ln)
		}
	}
	before := fmt.Sprint(sortedKeys(set))
	mut(set)
	if before == fmt.Sprint(sortedKeys(set)) {
		return content, false
	}
	for len(outside) > 0 && strings.TrimSpace(outside[len(outside)-1]) == "" {
		outside = outside[:len(outside)-1]
	}
	out := strings.Join(outside, "\n")
	if keys := sortedKeys(set); len(keys) > 0 {
		out += "\n\n" + begin + "\n"
		for _, k := range keys {
			out += Line(k) + "\n"
		}
		out += end
	}
	return out + "\n", true
}

func sortedKeys(m map[string]bool) []string {
	k := make([]string, 0, len(m))
	for s := range m {
		k = append(k, s)
	}
	sort.Strings(k)
	return k
}

func write(content string) error {
	err := os.WriteFile(Path(), []byte(content), 0o644)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrPermission) {
		return err
	}

	temp, createErr := os.CreateTemp("", "laraserve-hosts-*")
	if createErr != nil {
		return createErr
	}
	defer os.Remove(temp.Name())
	if _, createErr = temp.WriteString(content); createErr != nil {
		temp.Close()
		return createErr
	}
	if createErr = temp.Close(); createErr != nil {
		return createErr
	}
	cmd, authErr := elevate.Command("cp", temp.Name(), Path())
	if authErr != nil {
		return authErr
	}
	cmd.Stdin = nil
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err == nil {
		activity.Detail("Local domain configured")
	}
	return err
}
