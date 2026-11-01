package serve

import (
	"strings"
	"testing"
)

func TestCaddyfile(t *testing.T) {
	c := Config{Root: "/app", Domain: "testapp.test", Host: "127.0.0.1", Port: 8443, HTTPS: true,
		Cert: "/c.pem", Key: "/k.pem", AppEnv: map[string]string{"APP_URL": "https://testapp.test:8443"}}
	out := Caddyfile(c)
	for _, want := range []string{
		"https://testapp.test:8443, https://localhost:8443 {",
		`tls "/c.pem" "/k.pem"`, `root * "/app/public"`, "php_server {",
		`env APP_URL "https://testapp.test:8443"`, "auto_https off", "skip_install_trust",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	c.HTTPS, c.Port, c.AppEnv = false, 8080, nil
	out = Caddyfile(c)
	if strings.Contains(out, "tls ") || !strings.Contains(out, "http://testapp.test:8080") {
		t.Errorf("bad http config:\n%s", out)
	}
	if got := (Config{Domain: "a.test", Port: 443, HTTPS: true}).URL(); got != "https://a.test" {
		t.Errorf("URL = %s", got)
	}
}

func TestPHPMyAdminStaysOnLoopback(t *testing.T) {
	c := Config{Root: "/app", Host: "0.0.0.0", Domain: "localhost", LAN: true, Port: 8080,
		Admin: &AdminConfig{Root: "/private/phpmyadmin", Port: 8081}}
	out := Caddyfile(c)
	i := strings.Index(out, "http://localhost:8081 {")
	if i < 0 {
		t.Fatal("missing phpMyAdmin site")
	}
	admin := out[i:]
	if !strings.Contains(admin, "bind 127.0.0.1") || strings.Contains(admin, "bind 0.0.0.0") {
		t.Fatal("phpMyAdmin exposed on app LAN interface")
	}
	for _, want := range []string{`root * "/private/phpmyadmin"`, "/config.inc.php", "/libraries/*", "respond @privateFiles 404", "php_server"} {
		if !strings.Contains(admin, want) {
			t.Fatalf("admin site missing %s", want)
		}
	}
}

func TestNativePHPProxyConfiguration(t *testing.T) {
	c := Config{Root: "/app", Domain: "localhost", Host: "127.0.0.1", Port: 8443, HTTPS: true, Cert: "/ca.pem", Key: "/key.pem", BackendPort: 19000, Admin: &AdminConfig{Root: "/manager", Port: 8444, BackendPort: 19001}}
	out := Caddyfile(c)
	for _, want := range []string{"reverse_proxy 127.0.0.1:19000", "reverse_proxy 127.0.0.1:19001", "bind 127.0.0.1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %s", want)
		}
	}
	if strings.Contains(out, "php_server") {
		t.Fatal("native project would use embedded PHP")
	}
}
