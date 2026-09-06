package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/megashchik/migrate/config"
)

func TestCheckHappy(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "000001_a.sql")
	writeTestFile(t, dir, "000002_b.sql")

	if err := Check(&config.Config{Dir: dir}); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheckEmptyDir(t *testing.T) {
	if err := Check(&config.Config{Dir: t.TempDir()}); err != nil {
		t.Fatalf("Check() error: %v", err)
	}
}

func TestCheckDetectsDuplicates(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "000001_a.sql")
	writeTestFile(t, dir, "000001_b.sql")
	writeTestFile(t, dir, "000002_ok.sql")

	c := &config.Config{Dir: dir}

	var checkErr error
	out, err := captureStdout(func() { checkErr = Check(c) })
	if err != nil {
		t.Fatalf("captureStdout error: %v", err)
	}

	// filenames are printed to stdout; the error carries only the count
	for _, want := range []string{"000001_a.sql", "000001_b.sql"} {
		if !strings.Contains(out, want) {
			t.Errorf("Check() stdout %q missing %q", out, want)
		}
	}
	if checkErr == nil || !strings.Contains(checkErr.Error(), "duplicate") {
		t.Fatalf("Check() error = %v, want duplicate message", checkErr)
	}
}

func TestCheckPrintsOkOnClean(t *testing.T) {
	dir := t.TempDir()
	out, err := captureStdout(func() { _ = Check(&config.Config{Dir: dir}) })
	if err != nil {
		t.Fatalf("captureStdout error: %v", err)
	}
	if !strings.Contains(out, "ok: no duplicate migration versions") {
		t.Errorf("Check() output = %q, want ok message", out)
	}
}

func TestNewCreatesMigrationFile(t *testing.T) {
	dir := t.TempDir()
	c := &config.Config{Dir: dir, Format: "0", CommandArg: "create_users"}

	if err := New(c); err != nil {
		t.Fatalf("New() error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "000001_create_users.sql"))
	if err != nil {
		t.Fatalf("migration file not created: %v", err)
	}
	if len(content) != 0 {
		t.Errorf("migration content = %q, want empty", content)
	}
}

func TestNewWritesDescriptionComment(t *testing.T) {
	dir := t.TempDir()
	c := &config.Config{Dir: dir, Format: "0", Desc: true, CommandArg: "add_phone"}

	if err := New(c); err != nil {
		t.Fatalf("New() error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "000001_add_phone.sql"))
	if err != nil {
		t.Fatalf("migration file not created: %v", err)
	}
	if want := "-- desc: add_phone\n"; string(content) != want {
		t.Errorf("migration content = %q, want %q", content, want)
	}
}

func TestNewCreatesMissingDir(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "nested", "migrations")

	if err := New(&config.Config{Dir: dir, Format: "0", CommandArg: "x"}); err != nil {
		t.Fatalf("New() error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "000001_x.sql")); err != nil {
		t.Errorf("expected migration created in new dir: %v", err)
	}
}

func TestHelp(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		out, err := captureStdout(func() { Help(&config.Config{}) })
		if err != nil {
			t.Fatalf("captureStdout error: %v", err)
		}
		// `new <name>` is only listed in the -extra (advanced) help
		for _, want := range []string{"migrate", "Usage:"} {
			if !strings.Contains(out, want) {
				t.Errorf("Help() output missing %q", want)
			}
		}
	})

	t.Run("extra", func(t *testing.T) {
		out, err := captureStdout(func() { Help(&config.Config{Extra: true}) })
		if err != nil {
			t.Fatalf("captureStdout error: %v", err)
		}
		for _, want := range []string{"Advanced Features", "Timestamp:", "check", "YYYYMMDDHHMMSSmmm"} {
			if !strings.Contains(out, want) {
				t.Errorf("Help(-extra) output missing %q", want)
			}
		}
	})
}

func TestVersion(t *testing.T) {
	out, err := captureStdout(func() { Version() })
	if err != nil {
		t.Fatalf("captureStdout error: %v", err)
	}
	if !strings.Contains(out, "migrate version") {
		t.Errorf("Version() output = %q, missing version string", out)
	}
}

func TestGetDBRequiresConnection(t *testing.T) {
	_, err := getDB(&config.Config{})
	if err == nil || !strings.Contains(err.Error(), "please provide a conn string") {
		t.Fatalf("getDB() = %v, want 'please provide a conn string'", err)
	}
}

func TestGetDBPingFailure(t *testing.T) {
	// the in-memory driver (fakedb) routes "ping-fail" DSNs to a refused Ping
	c := &config.Config{Conn: "memory://ping-fail/db?sslmode=disable"}
	_, err := getDB(c)
	if err == nil || !strings.Contains(err.Error(), "failed to connect") {
		t.Fatalf("getDB() = %v, want 'failed to connect'", err)
	}
}
