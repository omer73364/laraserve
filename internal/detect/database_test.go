package detect

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDatabaseConnection(t *testing.T) {
	for _, test := range []struct{ name, env, config, want string }{
		{"sqlite fallback", "", "", "sqlite"},
		{"mysql env", "DB_CONNECTION=mysql\n", "", "mysql"},
		{"quoted mariadb", "DB_CONNECTION=\"mariadb\"\n", "", "mariadb"},
		{"url precedence", "DB_CONNECTION=sqlite\nDB_URL=mysql://user:password@localhost/app\n", "", "mysql"},
		{"external postgres", "DB_URL=postgres://localhost/app\n", "", "postgres"},
		{"framework default", "", "<?php return ['default' => env('DB_CONNECTION', 'mysql')];", "mysql"},
		{"literal default", "", `<?php return ["default" => "mysql"];`, "mysql"},
		{"env overrides default", "DB_CONNECTION=sqlite\n", "<?php return ['default' => env('DB_CONNECTION', 'mysql')];", "sqlite"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(test.env), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(dir, "config"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "config", "database.php"), []byte(test.config), 0o600); err != nil {
				t.Fatal(err)
			}
			if got := DatabaseConnection(dir); got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}
