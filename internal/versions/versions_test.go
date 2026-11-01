package versions

import (
	"github.com/Masterminds/semver/v3"
	"os"
	"path/filepath"
	"testing"
)

var phpVersions = []string{"7.4.33", "8.0.30", "8.1.34", "8.2.35", "8.3.36", "8.4.27", "8.5.12"}

func file(t *testing.T, root, name, body string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestPHPProjectSelection(t *testing.T) {
	for _, tc := range []struct{ name, composer, pin, lock, override, want string }{
		{name: "default", want: "8.5.12"},
		{name: "Laravel 8 PHP range", composer: `{"require":{"php":"^7.3|^8.0","laravel/framework":"^8.75"}}`, want: "7.4.33"},
		{name: "legacy PHP pin", composer: `{"require":{"php":"^7.3|^8.0"}}`, pin: "8.0", want: "8.0.30"},
		{name: "old dependency excludes PHP 8", composer: `{"require":{"php":"^7.4|^8.0"}}`, lock: `{"packages":[{"require":{"php":"^7.2"}}]}`, want: "7.4.33"},
		{name: "PHP 8 minimum", composer: `{"require":{"php":"^8.0"}}`, want: "8.0.30"},
		{name: "PHP 8.1 pin", pin: "8.1", want: "8.1.34"},
		{name: "minimum supported minor", composer: `{"require":{"php":"^8.2"}}`, want: "8.2.35"},
		{name: "dependency raises requirement", composer: `{"require":{"php":"^8.2"}}`, lock: `{"packages":[{"require":{"php":"^8.3"}}]}`, want: "8.3.36"},
		{name: "platform selects minor", composer: `{"require":{"php":"^8.2"},"config":{"platform":{"php":"8.4.0"}}}`, want: "8.4.27"},
		{name: "explicit pin", composer: `{"require":{"php":"^8.2"}}`, pin: "8.3", want: "8.3.36"},
		{name: "cli wins", pin: "8.2", override: "8.4", want: "8.4.27"},
		{name: "incompatible pin rejected", composer: `{"require":{"php":"~8.3.0"}}`, pin: "8.5", want: ""},
		{name: "conflict excludes branch", composer: `{"require":{"php":"^8.2"},"conflict":{"php":"8.2.*"}}`, want: "8.3.36"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.composer != "" {
				file(t, root, "composer.json", tc.composer)
			}
			if tc.pin != "" {
				file(t, root, ".php-version", tc.pin)
			}
			if tc.lock != "" {
				file(t, root, "composer.lock", tc.lock)
			}
			v, err := ProjectPHP(root, phpVersions, tc.override, "")
			if tc.want == "" {
				if err == nil {
					t.Fatal("accepted conflicting pin")
				}
				return
			}
			if err != nil || v != tc.want {
				t.Fatalf("got %s %v, want %s", v, err, tc.want)
			}
		})
	}
}

func TestComposerConstraintSemantics(t *testing.T) {
	for _, tc := range []struct {
		constraint, version string
		want                bool
	}{
		{"~8.2", "8.5.12", true}, {"~8.2.0", "8.3.36", false}, {"8.2.*", "8.2.35", true}, {"8.2", "8.2.35", false}, {">=8.2 <8.4", "8.3.36", true}, {"^7.4|^8.2", "8.4.27", true}, {"8.2 - 8.4", "8.4.27", true},
	} {
		c, err := ComposerConstraint(tc.constraint)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := semver.NewVersion(tc.version)
		if got := c.Check(v); got != tc.want {
			t.Fatalf("%s with %s: %v", tc.constraint, tc.version, got)
		}
	}
}

func TestMySQLProjectSelection(t *testing.T) {
	available := []string{"8.0.46", "8.4.13"}
	for _, tc := range []struct{ name, pin, env, compose, override, want string }{
		{name: "default", want: "8.4.13"}, {name: "file", pin: "8.0", want: "8.0.46"}, {name: "env", env: "MYSQL_VERSION=8.0\n", want: "8.0.46"},
		{name: "compose", compose: "services:\n  mysql:\n    image: mysql:8.0\n", want: "8.0.46"},
		{name: "compose default", compose: "services:\n  mysql:\n    image: mysql:${MYSQL_VERSION:-8.0}\n", want: "8.0.46"},
		{name: "explicit override", pin: "8.0", override: "8.4", want: "8.4.13"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.pin != "" {
				file(t, root, ".mysql-version", tc.pin)
			}
			file(t, root, ".env", tc.env)
			file(t, root, "compose.yaml", tc.compose)
			v, err := ProjectMySQL(root, available, tc.override, "")
			if err != nil || v != tc.want {
				t.Fatalf("got %s %v", v, err)
			}
		})
	}
}

func TestCompatibleLockIsStable(t *testing.T) {
	root := t.TempDir()
	file(t, root, "composer.json", `{"require":{"php":"^8.2"}}`)
	v, err := ProjectPHP(root, phpVersions, "", "8.4.27")
	if err != nil || v != "8.4.27" {
		t.Fatal(v, err)
	}
	file(t, root, ".php-version", "8.3")
	v, err = ProjectPHP(root, phpVersions, "", "8.4.27")
	if err != nil || v != "8.3.36" {
		t.Fatal(v, err)
	}
}
