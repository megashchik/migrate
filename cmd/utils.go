package cmd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/megashchik/migrate/config"
)

type fileVersion struct {
	file    string
	version int64
}

// getQuery builds the SELECT query and scan targets for the last/list commands.
func getQuery(c *config.Config) (string, []any, error) {
	var version int64

	var description sql.NullString

	var ts sql.NullTime

	values := []any{&version}
	params := []string{"version"}

	if c.Desc {
		params = append(params, "description")
		values = append(values, &description)
	}

	if c.Ts {
		params = append(params, "applied_at")
		values = append(values, &ts)
	}

	switch c.Command {
	case config.CommandLast:
		return fmt.Sprintf("SELECT %s FROM %s ORDER BY version DESC LIMIT 1", strings.Join(params, ", "), c.FullTableName), values, nil
	case config.CommandList:
		return fmt.Sprintf("SELECT %s FROM %s ORDER BY version DESC", strings.Join(params, ", "), c.FullTableName), values, nil
	default:
		return "", nil, fmt.Errorf("unknown command: %s", c.Command)
	}
}

// formatRow joins scanned migration values into a readable line.
func formatRow(values []any) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, formatValue(v))
	}

	return strings.Join(parts, "  ")
}

// formatValue returns a readable representation of a scanned migration column.
func formatValue(v any) string {
	switch t := v.(type) {
	case *int64:
		return strconv.FormatInt(*t, 10)
	case *sql.NullString:
		if t.Valid {
			return t.String
		}
	case *sql.NullTime:
		if t.Valid {
			return t.Time.UTC().Format(time.RFC3339)
		}
	}

	return "-"
}

// getLastVersion returns the last migration version in the directory.
func getLastVersion(dir string) (int64, error) {
	_, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return 0, fmt.Errorf("failed to get files: %w", err)
	}

	if len(files) == 0 {
		return 0, nil
	}

	var maxVersion int64 = 0

	for _, file := range files {
		version, err := getVersion(file)
		if err != nil {
			return 0, err
		}

		if version > maxVersion {
			maxVersion = version
		}
	}

	return maxVersion, nil
}

// getVersion returns the version of a migration file.
func getVersion(filename string) (int64, error) {
	nameWithoutExt := strings.TrimSuffix(filepath.Base(filename), ".sql")

	before, _, _ := strings.Cut(nameWithoutExt, "_")

	version, err := strconv.ParseInt(before, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("can't get version from filename %s: %w", filename, err)
	}

	return version, nil
}

// getDB returns a database connection.
func getDB(c *config.Config) (*sql.DB, error) {
	if c.Conn == "" {
		return nil, errors.New("please provide a conn string using -conn=postgres://user:password@host:port/database?sslmode=disable")
	}

	db, err := sql.Open("postgres", c.Conn)
	if err != nil {
		return nil, fmt.Errorf("failed to open connect to database: %w", err)
	}

	err = db.PingContext(context.Background())
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return db, nil
}

// closeDb closes the database connection.
func closeDb(db *sql.DB) {
	err := db.Close()
	if err != nil {
		log.Printf("failed to close db: %s\n", err)
	}
}

// getMigrationFiles returns the migration files in dir with their versions.
func getMigrationFiles(dir string) ([]fileVersion, error) {
	files, err := filepath.Glob(dir + "/*.sql")
	if err != nil {
		return nil, fmt.Errorf("failed to get files: %w", err)
	}

	migrations := make([]fileVersion, 0, len(files))
	for _, f := range files {
		version, err := getVersion(f)
		if err != nil {
			return nil, err
		}

		migrations = append(migrations, fileVersion{f, version})
	}

	return migrations, nil
}

// duplicateVersions returns a sorted list of versions that appear more than once.
func duplicateVersions(migrations []fileVersion) []string {
	byVersion := make(map[int64][]string)
	for _, m := range migrations {
		byVersion[m.version] = append(byVersion[m.version], m.file)
	}

	var dupes []string
	for version, files := range byVersion {
		if len(files) > 1 {
			dupes = append(dupes, fmt.Sprintf("%d: %s", version, strings.Join(files, ", ")))
		}
	}

	slices.Sort(dupes)

	return dupes
}
