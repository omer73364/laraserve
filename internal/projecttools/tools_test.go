package projecttools

import (
	"context"
	"laraserve/internal/statefile"
	"laraserve/internal/supervisor"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"laraserve/internal/projectconfig"
	"laraserve/internal/projects"
	"laraserve/internal/runtime"
)

func TestPrivatePHPConfigurationDoesNotModifyCache(t *testing.T) {
	base, state := t.TempDir(), t.TempDir()
	original := "extension_dir=/private/extensions\nmemory_limit=512M\n"
	os.WriteFile(filepath.Join(base, "php.ini"), []byte(original), 0600)
	rt := &runtime.Runtime{INI: base}
	if err := ConfigurePHP(rt, state, projectconfig.Config{PHPSettings: map[string]string{"memory_limit": "192M"}}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(base, "php.ini"))
	if string(b) != original {
		t.Fatal("shared ini changed")
	}
	b, _ = os.ReadFile(filepath.Join(rt.INI, "php.ini"))
	if !strings.Contains(string(b), "memory_limit = 192M") {
		t.Fatal(string(b))
	}
}
func TestRuntimeCleanupProtectsReferencesAndData(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	key := "php-unused-aaaaaaaaaaaaaaaaaaaa"
	kept := "php-selected-bbbbbbbbbbbbbbbbbbbb"
	for _, name := range []string{key, kept} {
		dir := filepath.Join(home, "runtimes", name)
		os.MkdirAll(dir, 0700)
		os.WriteFile(filepath.Join(dir, ".ready"), []byte("ready"), 0600)
		os.WriteFile(filepath.Join(dir, "frankenphp"), []byte("runtime"), 0600)
	}
	p := &projects.Project{ID: "abcdef123456", Name: "test", Source: root, Root: root}
	projects.New(home).Save(p)
	if err := RemoveUnusedCache(context.Background(), home, key); err == nil {
		t.Fatal("cleanup allowed without reference snapshot")
	}
	SaveExecution(home, p.ID, Execution{Runtime: runtime.Runtime{Dir: filepath.Join(home, "runtimes", kept), PHP: filepath.Join(home, "runtimes", kept, "frankenphp")}})
	database := filepath.Join(home, "databases", "sentinel")
	os.MkdirAll(filepath.Dir(database), 0700)
	os.WriteFile(database, []byte("keep"), 0600)
	if err := RemoveUnusedCache(context.Background(), home, kept); err == nil {
		t.Fatal("referenced cache removed")
	}
	if err := RemoveUnusedCache(context.Background(), home, key); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "runtimes", key)); !os.IsNotExist(err) {
		t.Fatal("unused cache remains")
	}
	if b, _ := os.ReadFile(database); string(b) != "keep" {
		t.Fatal("database changed")
	}
}
func TestTaskValidationRequiresDeliberateSchemaConsent(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	id := "abcdef123456"
	projects.New(home).Save(&projects.Project{ID: id, Source: root, Root: root})
	for _, kind := range []string{"artisan", "composer"} {
		r := TaskRequest{ProjectID: id, Kind: kind, Args: []string{"migrate"}}
		if err := validateTask(home, r); err == nil {
			t.Fatal("unconfirmed/unsupported action accepted")
		}
	}
	if err := validateTask(home, TaskRequest{ProjectID: id, Kind: "artisan", Args: []string{"migrate"}, ConfirmSchema: true}); err != nil {
		t.Fatal(err)
	}
	if err := validateTask(home, TaskRequest{ProjectID: id, Kind: "composer", Args: []string{"install", "--working-dir=/etc"}}); err == nil {
		t.Fatal("arbitrary composer directory accepted")
	}
}

func TestSettingsPromoteRememberedServiceOptions(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	p := &projects.Project{ID: "abcdef123456", Source: root, Root: root, Args: []string{root, "--http=true", "--mysql=false", "--phpmyadmin=false", "--php-version=8.5"}}
	if err := projects.New(home).Save(p); err != nil {
		t.Fatal(err)
	}
	doc, err := ReadSettings(p)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Config.Database != "external" || doc.Config.HTTPS == nil || *doc.Config.HTTPS || doc.Config.PHPMyAdmin == nil || *doc.Config.PHPMyAdmin {
		t.Fatal("remembered options missing", doc)
	}
	// The fixture has no Composer constraint, so the catalog pin can be checked.
	doc.Config.PHPSettings = map[string]string{"memory_limit": "192M"}
	if _, err = SaveSettings(context.Background(), home, p, doc.Revision, doc.Config); err != nil {
		t.Fatal(err)
	}
	saved, err := projectconfig.Read(root)
	if err != nil || saved.Config.Database != "external" {
		t.Fatal(saved, err)
	}
	record, err := projects.New(home).Load(p.ID)
	if err != nil || len(record.Args) != 1 {
		t.Fatal(record, err)
	}
}

func TestInterruptedTasksHoldFilesUntilOwnedCleanupCanFinish(t *testing.T) {
	home := t.TempDir()
	id := "abcdef123456"
	os.MkdirAll(filepath.Join(taskDir(home), "control"), 0700)
	task := Task{ID: id, ProjectID: "123456abcdef", Kind: "composer", Status: "running", Started: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)}
	statefile.WriteJSON(taskFile(home, id), task)
	statefile.WriteJSON(filepath.Join(taskDir(home), "control", id+".json"), supervisor.State{ID: id, Generation: "previous", Status: "running", Address: "127.0.0.1:1", Token: supervisor.NewID() + supervisor.NewID()})
	current, err := readTask(home, id, false)
	if err != nil || current.Status != "recovering" {
		t.Fatal(current, err)
	}
	if err = RequireNoTasks(home, task.ProjectID); err == nil {
		t.Fatal("cleanup allowed before child grace period")
	}
	past := time.Now().Add(-26 * time.Second)
	os.Chtimes(filepath.Join(taskDir(home), id+".interrupted"), past, past)
	current, err = readTask(home, id, false)
	if err != nil || current.Status != "interrupted" {
		t.Fatal(current, err)
	}
	if err = RequireNoTasks(home, task.ProjectID); err != nil {
		t.Fatal(err)
	}
}

func TestNPMTasksRejectPersistentAndArbitraryCommands(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	id := "abcdef123456"
	if err := projects.New(home).Save(&projects.Project{ID: id, Source: root, Root: root}); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"install"}, {"ci"}, {"run", "build"}, {"audit"}, {"outdated"}} {
		if err := validateTask(home, TaskRequest{ProjectID: id, Kind: "npm", Args: args}); err != nil {
			t.Fatal(args, err)
		}
	}
	for _, args := range [][]string{{"run", "dev"}, {"exec", "anything"}, {"install", "--prefix=/tmp/other"}, {"run", "build", "--", "--watch"}} {
		if err := validateTask(home, TaskRequest{ProjectID: id, Kind: "npm", Args: args}); err == nil {
			t.Fatal("unexpected npm command accepted", args)
		}
	}
}
