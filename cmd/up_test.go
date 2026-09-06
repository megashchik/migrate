package cmd

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestGetMigrationFiles(t *testing.T) {
	t.Run("empty dir returns empty slice", func(t *testing.T) {
		got, err := getMigrationFiles(t.TempDir())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("getMigrationFiles = %v, want empty", got)
		}
	})

	t.Run("returns files with versions", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "000002_b.sql")
		writeTestFile(t, dir, "000001_a.sql")

		got, err := getMigrationFiles(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		want := []fileVersion{
			{file: filepath.Join(dir, "000001_a.sql"), version: 1},
			{file: filepath.Join(dir, "000002_b.sql"), version: 2},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("getMigrationFiles = %v, want %v", got, want)
		}
	})

	t.Run("invalid file errors", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "not_a_version.sql")

		if _, err := getMigrationFiles(dir); err == nil {
			t.Fatal("expected error for invalid filename, got nil")
		}
	})

	t.Run("ignores non-sql files", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, dir, "000001_ok.sql")
		writeTestFile(t, dir, "README.md")

		got, err := getMigrationFiles(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Errorf("getMigrationFiles = %v, want only the one .sql file", got)
		}
	})
}

func TestDuplicateVersions(t *testing.T) {
	t.Run("no duplicates returns empty", func(t *testing.T) {
		migs := []fileVersion{
			{file: "/dir/000001_a.sql", version: 1},
			{file: "/dir/000002_b.sql", version: 2},
		}
		if got := duplicateVersions(migs); len(got) != 0 {
			t.Errorf("duplicateVersions = %v, want empty", got)
		}
	})

	t.Run("empty slice returns empty", func(t *testing.T) {
		if got := duplicateVersions(nil); len(got) != 0 {
			t.Errorf("duplicateVersions(nil) = %v, want empty", got)
		}
	})

	t.Run("reports both files for a duplicate", func(t *testing.T) {
		migs := []fileVersion{
			{file: "/dir/000001_a.sql", version: 1},
			{file: "/dir/000001_b.sql", version: 1},
		}
		got := duplicateVersions(migs)
		if len(got) != 1 {
			t.Fatalf("duplicateVersions = %v, want 1 entry", got)
		}
		for _, want := range []string{"/dir/000001_a.sql", "/dir/000001_b.sql"} {
			if !strings.Contains(got[0], want) {
				t.Errorf("duplicate entry %q missing %q", got[0], want)
			}
		}
	})

	t.Run("multiple duplicate groups are sorted", func(t *testing.T) {
		migs := []fileVersion{
			{file: "/dir/000002_x.sql", version: 2},
			{file: "/dir/000001_a.sql", version: 1},
			{file: "/dir/000001_b.sql", version: 1},
			{file: "/dir/000002_y.sql", version: 2},
		}
		got := duplicateVersions(migs)
		if len(got) != 2 {
			t.Fatalf("duplicateVersions = %v, want 2 entries", got)
		}
		if !strings.HasPrefix(got[0], "1:") {
			t.Errorf("first entry %q should start with version 1", got[0])
		}
		if !strings.HasPrefix(got[1], "2:") {
			t.Errorf("second entry %q should start with version 2", got[1])
		}
	})
}
