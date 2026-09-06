package cmd

import (
	"database/sql/driver"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/megashchik/migrate/config"
)

// integrationConfig returns a config backed by a fresh, isolated in-memory
// database (see fakedb_test.go). The DSN is unique per test so stores never
// leak state across tests.
func integrationConfig(t *testing.T, dir string) *config.Config {
	t.Helper()

	table := fmt.Sprintf("schema_mig_%d", time.Now().UnixNano())

	return &config.Config{
		Conn:          "memory://" + table,
		Dir:           dir,
		Schema:        "public",
		Table:         table,
		FullTableName: fmt.Sprintf(`"public"."%s"`, table),
		Format:        "T",
	}
}

func writeSQL(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write migration %s: %v", name, err)
	}
}

// store returns the in-memory database bound to the config's DSN.
func store(t *testing.T, c *config.Config) *dbState {
	t.Helper()
	return baseStore(c.Conn)
}

func metaRows(t *testing.T, c *config.Config) []map[string]driver.Value {
	t.Helper()
	s := store(t, c)
	s.mu.Lock()
	defer s.mu.Unlock()
	ts, ok := s.tables[tableKey(c.Table)]
	if !ok {
		t.Fatalf("metadata table %q not created", c.Table)
	}

	return ts.Rows()
}

func rowCount(t *testing.T, c *config.Config, table string) int {
	t.Helper()
	s := store(t, c)
	s.mu.Lock()
	defer s.mu.Unlock()
	ts, ok := s.tables[tableKey(table)]
	if !ok {
		t.Fatalf("table %q not found", table)
	}

	return ts.RowCount()
}

// TestMemUpRejectsDuplicateVersions covers Up's own duplicate validation
// (up.go's duplicateVersions branch). test-integration.sh exercises duplicate
// detection only through the separate `check` command, so this Up path is not
// duplicated by it.
func TestMemUpRejectsDuplicateVersions(t *testing.T) {
	c := integrationConfig(t, t.TempDir())

	writeSQL(t, c.Dir, "20000101000000000_a.sql", "SELECT 1;")
	writeSQL(t, c.Dir, "20000101000000000_b.sql", "SELECT 2;")

	err := Up(c)
	if err == nil || !strings.Contains(err.Error(), "duplicate migration versions") {
		t.Fatalf("Up() error = %v, want 'duplicate migration versions'", err)
	}
}

// TestMemUpIdempotentReRun exercises the skip-already-applied path: a second
// Up on the same config must read the applied versions and skip them. This
// branch (Up's continue / appliedVersions non-empty) is only reachable on a
// re-run, so it needs its own test.
func TestMemUpIdempotentReRun(t *testing.T) {
	c := integrationConfig(t, t.TempDir())
	writeSQL(t, c.Dir, "20000101000000000_first.sql", "SELECT 1;")
	writeSQL(t, c.Dir, "20000101000000001_second.sql", "SELECT 1;")

	if err := Up(c); err != nil {
		t.Fatalf("first Up() error: %v", err)
	}
	if n := rowCount(t, c, c.Table); n != 2 {
		t.Fatalf("applied count = %d, want 2", n)
	}

	// Second run: appliedVersions is non-empty, both migrations are skipped.
	if err := Up(c); err != nil {
		t.Fatalf("second Up() error: %v", err)
	}
	if n := rowCount(t, c, c.Table); n != 2 {
		t.Errorf("applied count after re-run = %d, want 2 (idempotent)", n)
	}
}

func TestMemUpFailsOnInvalidSQLAndLeavesNothingApplied(t *testing.T) {
	c := integrationConfig(t, t.TempDir())

	writeSQL(t, c.Dir, "20000101000000000_bad.sql", "THIS IS NOT VALID SQL;")

	err := Up(c)
	if err == nil || !strings.Contains(err.Error(), "failed to execute migration file") {
		t.Fatalf("Up() error = %v, want 'failed to execute migration file'", err)
	}

	if n := rowCount(t, c, c.Table); n != 0 {
		t.Errorf("applied count = %d, want 0 (transaction rolled back)", n)
	}
}

func TestMemCreateTableColumnTypes(t *testing.T) {
	t.Run("bigint by default", func(t *testing.T) {
		c := integrationConfig(t, t.TempDir())
		if err := Up(c); err != nil {
			t.Fatalf("Up() error: %v", err)
		}
		if got := store(t, c).columnType(c.Table, "version").typ; got != "bigint" {
			t.Errorf("version column = %q, want bigint", got)
		}
	})

	t.Run("short uses integer", func(t *testing.T) {
		c := integrationConfig(t, t.TempDir())
		c.Short = true
		if err := Up(c); err != nil {
			t.Fatalf("Up() error: %v", err)
		}
		if got := store(t, c).columnType(c.Table, "version").typ; got != "integer" {
			t.Errorf("version column = %q, want integer", got)
		}
	})

	t.Run("desc and ts add columns", func(t *testing.T) {
		c := integrationConfig(t, t.TempDir())
		c.Desc = true
		c.Ts = true
		if err := Up(c); err != nil {
			t.Fatalf("Up() error: %v", err)
		}
		if got := store(t, c).columnType(c.Table, "description").typ; got != "text" {
			t.Errorf("description column = %q, want text", got)
		}
		if got := store(t, c).columnType(c.Table, "applied_at").typ; got != "timestamp with time zone" {
			t.Errorf("applied_at column = %q, want timestamp with time zone", got)
		}
	})
}

func TestMemUpExtractsDescription(t *testing.T) {
	c := integrationConfig(t, t.TempDir())
	c.Desc = true

	writeSQL(t, c.Dir, "20000101000000000_desc.sql", "-- desc: adds phone column\nSELECT 1;")
	writeSQL(t, c.Dir, "20000101000000001_named.sql", "SELECT 1;")

	if err := Up(c); err != nil {
		t.Fatalf("Up() error: %v", err)
	}

	rows := metaRows(t, c)
	descByVersion := map[string]string{}
	for _, r := range rows {
		v := strconv.FormatInt(asVersion(r["version"]), 10)
		descByVersion[v] = asString(r["description"])
	}

	if descByVersion["20000101000000000"] != "adds phone column" {
		t.Errorf("description from comment = %q, want 'adds phone column'", descByVersion["20000101000000000"])
	}
	if descByVersion["20000101000000001"] != "named" {
		t.Errorf("description from filename = %q, want 'named'", descByVersion["20000101000000001"])
	}
}

func TestMemUpPopulatesAppliedAtWhenEnabled(t *testing.T) {
	c := integrationConfig(t, t.TempDir())
	c.Ts = true

	writeSQL(t, c.Dir, "20000101000000000_ts.sql", "SELECT 1;")

	if err := Up(c); err != nil {
		t.Fatalf("Up() error: %v", err)
	}

	rows := metaRows(t, c)
	if len(rows) != 1 {
		t.Fatalf("expected 1 applied migration, got %d", len(rows))
	}
	if _, ok := rows[0]["applied_at"].(time.Time); !ok {
		t.Errorf("applied_at = %T (%v), want a time", rows[0]["applied_at"], rows[0]["applied_at"])
	}
}

func TestMemLastSuccessPath(t *testing.T) {
	c := integrationConfig(t, t.TempDir())
	c.Command = config.CommandLast

	writeSQL(t, c.Dir, "20000101000000000_v.sql", "SELECT 1;")
	if err := Up(c); err != nil {
		t.Fatalf("Up() error: %v", err)
	}

	if err := Last(c); err != nil {
		t.Fatalf("Last() error: %v", err)
	}
}

func TestMemListSuccessPath(t *testing.T) {
	c := integrationConfig(t, t.TempDir())
	c.Command = config.CommandList

	writeSQL(t, c.Dir, "20000101000000000_v.sql", "SELECT 1;")
	writeSQL(t, c.Dir, "20000101000000001_w.sql", "SELECT 1;")
	if err := Up(c); err != nil {
		t.Fatalf("Up() error: %v", err)
	}

	if err := List(c); err != nil {
		t.Fatalf("List() error: %v", err)
	}
}

func TestMemLastEmptyTablePrintsNotice(t *testing.T) {
	c := integrationConfig(t, t.TempDir())
	c.Command = config.CommandLast

	// Up on an empty dir creates the metadata table without rows.
	if err := Up(c); err != nil {
		t.Fatalf("Up() error: %v", err)
	}

	out, err := captureStdout(func() { _ = Last(c) })
	if err != nil {
		t.Fatalf("captureStdout error: %v", err)
	}
	if out != "no migrations applied yet\n" {
		t.Errorf("Last() output = %q, want 'no migrations applied yet\\n'", out)
	}
}

func TestMemLastErrorsWhenTableMissing(t *testing.T) {
	c := integrationConfig(t, t.TempDir())
	c.Command = config.CommandLast

	err := Last(c)
	if err == nil || !strings.Contains(err.Error(), "failed to get last version") {
		t.Fatalf("Last() error = %v, want 'failed to get last version'", err)
	}
}

func TestMemListErrorsWhenTableMissing(t *testing.T) {
	c := integrationConfig(t, t.TempDir())
	c.Command = config.CommandList

	err := List(c)
	if err == nil || !strings.Contains(err.Error(), "failed db query") {
		t.Fatalf("List() error = %v, want 'failed db query'", err)
	}
}

func TestMemUpAppliesInSourceOrder(t *testing.T) {
	c := integrationConfig(t, t.TempDir())
	c.Desc = true

	writeSQL(t, c.Dir, "20000101000000000_first.sql", "-- desc: first\nSELECT 1;")
	writeSQL(t, c.Dir, "20000101000000002_third.sql", "-- desc: third\nSELECT 1;")
	writeSQL(t, c.Dir, "20000101000000001_second.sql", "-- desc: second\nSELECT 1;")

	if err := Up(c); err != nil {
		t.Fatalf("Up() error: %v", err)
	}

	rows := metaRows(t, c)
	got := make([]string, 0, len(rows))
	for _, r := range rows {
		got = append(got, strconv.FormatInt(asVersion(r["version"]), 10)+"/"+asString(r["description"]))
	}
	want := []string{
		"20000101000000000/first",
		"20000101000000001/second",
		"20000101000000002/third",
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("applied order = %v, want %v", got, want)
	}
}

func asVersion(v any) int64 {
	if n, ok := v.(int64); ok {
		return n
	}

	return 0
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}

	return ""
}
