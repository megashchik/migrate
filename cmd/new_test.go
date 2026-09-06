package cmd

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/megashchik/migrate/config"
)

func TestGetVersion(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     int64
		wantErr  bool
	}{
		{name: "with suffix", filename: "20260101120000_foo_bar.sql", want: 20260101120000},
		{name: "no underscore", filename: "000001.sql", want: 1},
		{name: "leading zeros", filename: "000002_create.sql", want: 2},
		{name: "17 digit ms version", filename: "20260906105144950_create.sql", want: 20260906105144950},
		{name: "non sql file", filename: "000001", want: 1},
		{name: "invalid prefix errors", filename: "abc_foo.sql", wantErr: true},
		{name: "empty prefix errors", filename: "_foo.sql", wantErr: true},
		{name: "out of int64 errors", filename: "99999999999999999999_foo.sql", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getVersion(tt.filename)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("getVersion(%q) expected error, got %d", tt.filename, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("getVersion(%q) unexpected error: %v", tt.filename, err)
			}
			if got != tt.want {
				t.Errorf("getVersion(%q) = %d, want %d", tt.filename, got, tt.want)
			}
		})
	}
}

func TestGetLastVersion(t *testing.T) {
	t.Run("missing dir returns zero", func(t *testing.T) {
		got, err := getLastVersion(filepath.Join(t.TempDir(), "does-not-exist"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 0 {
			t.Errorf("getLastVersion = %d, want 0", got)
		}
	})

	t.Run("empty dir returns zero", func(t *testing.T) {
		got, err := getLastVersion(t.TempDir())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 0 {
			t.Errorf("getLastVersion = %d, want 0", got)
		}
	})

	t.Run("returns max of files", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "000001_a.sql")
		writeTestFile(t, dir, "000003_c.sql")
		writeTestFile(t, dir, "000002_b.sql")

		got, err := getLastVersion(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != 3 {
			t.Errorf("getLastVersion = %d, want 3", got)
		}
	})

	t.Run("invalid filename errors", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "not_a_version.sql")

		if _, err := getLastVersion(dir); err == nil {
			t.Fatal("expected error for invalid filename, got nil")
		}
	})
}

func TestParseTimeVersion(t *testing.T) {
	version, err := parseTimeVersion()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	s := strconv.FormatInt(version, 10)
	if len(s) != 17 {
		t.Fatalf("parseTimeVersion() = %q, want 17 digits (YYYYMMDDHHMMSSmmm)", s)
	}

	base, err := strconv.ParseInt(s[:14], 10, 64)
	if err != nil {
		t.Fatalf("failed to parse 14-digit base: %v", err)
	}

	now, _ := strconv.ParseInt(time.Now().Format("20060102150405"), 10, 64)
	if d := base - now; d > 1 || d < -1 {
		t.Errorf("parseTimeVersion() base %d differs from now %d by %d (allowed +/-1s)", base, now, d)
	}
}

// mustGeneratePrefix calls generateVersionPrefix with a config pointed at dir.
func mustGeneratePrefix(t *testing.T, dir, format string) string {
	t.Helper()
	c := &config.Config{Dir: dir, Format: format}
	prefix, err := generateVersionPrefix(c)
	if err != nil {
		t.Fatalf("generateVersionPrefix(format=%q) error: %v", format, err)
	}
	return prefix
}

func TestGenerateVersionPrefix(t *testing.T) {
	t.Run("incremental zero padding 6", func(t *testing.T) {
		dir := t.TempDir()
		if got := mustGeneratePrefix(t, dir, "0"); got != "000001" {
			t.Errorf("format 0 empty dir = %q, want 000001", got)
		}

		writeTestFile(t, dir, "000001_prev.sql")
		if got := mustGeneratePrefix(t, dir, "0"); got != "000002" {
			t.Errorf("format 0 after 000001 = %q, want 000002", got)
		}
	})

	t.Run("custom zero width 4", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "0002_prev.sql")
		if got := mustGeneratePrefix(t, dir, "0000"); got != "0003" {
			t.Errorf("format 0000 = %q, want 0003", got)
		}
	})

	t.Run("timestamp T", func(t *testing.T) {
		prefix := mustGeneratePrefix(t, t.TempDir(), "T")
		if len(prefix) != 17 {
			t.Errorf("format T = %q, want 17 digits", prefix)
		}
		if _, err := strconv.ParseInt(prefix, 10, 64); err != nil {
			t.Errorf("format T = %q is not an int: %v", prefix, err)
		}
	})

	t.Run("empty format defaults to T", func(t *testing.T) {
		prefix := mustGeneratePrefix(t, t.TempDir(), "")
		if len(prefix) != 17 {
			t.Errorf("empty format = %q, want 17 digits", prefix)
		}
	})

	t.Run("unix epoch U", func(t *testing.T) {
		prefix := mustGeneratePrefix(t, t.TempDir(), "U")
		epoch, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil {
			t.Fatalf("format U = %q not an int: %v", prefix, err)
		}
		now := time.Now().Unix()
		if d := epoch - now; d > 2 || d < -2 {
			t.Errorf("format U epoch %d differs from now %d by %d (allowed +/-2s)", epoch, now, d)
		}
	})

	t.Run("unknown format falls back to T", func(t *testing.T) {
		prefix := mustGeneratePrefix(t, t.TempDir(), "X")
		if len(prefix) != 17 {
			t.Errorf("unknown format = %q, want 17 digits (default T)", prefix)
		}
	})

	t.Run("timestamp overflow uses lastVersion+1", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "9000000000000000000_future.sql")

		got := mustGeneratePrefix(t, dir, "T")
		if got != "9000000000000000001" {
			t.Errorf("format T overflow = %q, want 9000000000000000001", got)
		}
	})

	t.Run("incremental padding overflow keeps value", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "000999999_big.sql")

		got := mustGeneratePrefix(t, dir, "0")
		if got != "1000000" {
			t.Errorf("format 0 padding overflow = %q, want 1000000", got)
		}
	})
}

func writeTestFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}
}

// small sanity guard that prefixes are kept as pure digit strings (no padding glitches).
func TestGenerateVersionPrefixIsNumeric(t *testing.T) {
	for _, f := range []string{"0", "0000", "T", "U", ""} {
		prefix := mustGeneratePrefix(t, t.TempDir(), f)
		if prefix == "" || strings.Trim(prefix, "0123456789") != "" {
			t.Errorf("format %q produced non-numeric prefix %q", f, prefix)
		}
	}
}
