package hosts

import (
	"strings"
	"testing"
)

func TestRewrite(t *testing.T) {
	orig := "127.0.0.1 localhost\n::1 localhost\n"
	a, ch := Rewrite(orig, func(s map[string]bool) { s["testapp.test"] = true })
	if !ch || !strings.Contains(a, "# BEGIN laraserve\n127.0.0.1 testapp.test\n# END laraserve") {
		t.Fatalf("add failed:\n%s", a)
	}
	b, ch := Rewrite(a, func(s map[string]bool) { s["testapp.test"] = true })
	if ch || b != a {
		t.Fatal("adding twice must be a no-op")
	}
	c, _ := Rewrite(a, func(s map[string]bool) { s["other.test"] = true })
	if strings.Count(c, "BEGIN laraserve") != 1 || !strings.Contains(c, "other.test") {
		t.Fatalf("second add failed:\n%s", c)
	}
	d, _ := Rewrite(c, func(s map[string]bool) { delete(s, "testapp.test"); delete(s, "other.test") })
	if d != orig {
		t.Fatalf("remove must restore original, got:\n%q", d)
	}
}
