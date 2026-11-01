package cli

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

func TestManagedMySQLSelection(t *testing.T) {
	for _, test := range []struct {
		connection string
		args       []string
		want       bool
	}{
		{"mysql", nil, true}, {"mariadb", nil, true}, {"sqlite", nil, false}, {"pgsql", nil, false},
		{"mysql", []string{"--mysql=false"}, false}, {"sqlite", []string{"--mysql"}, true},
	} {
		fs := flag.NewFlagSet("serve", flag.ContinueOnError)
		value := fs.Bool("mysql", false, "")
		if err := fs.Parse(test.args); err != nil {
			t.Fatal(err)
		}
		if got := managedMySQL(fs, *value, test.connection); got != test.want {
			t.Fatalf("connection %s args %v: got %v", test.connection, test.args, got)
		}
	}
}

func TestMySQLVersionsKeepSeparateData(t *testing.T) {
	base := t.TempDir()
	a := mysqlDataDir(base, "8.0.46")
	b := mysqlDataDir(base, "8.4.13")
	if a == b {
		t.Fatal("different MySQL versions share data")
	}
	if err := os.Mkdir(filepath.Join(base, "data"), 0700); err != nil {
		t.Fatal(err)
	}
	if mysqlDataDir(base, "8.4.13") != base {
		t.Fatal("existing 8.4 database lost")
	}
	if mysqlDataDir(base, "8.0.46") == base {
		t.Fatal("8.0 would open existing 8.4 data")
	}
}
