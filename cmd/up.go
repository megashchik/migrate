package cmd

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/megashchik/migrate/config"
)

type fileVersion struct {
	file    string
	version int64
}

// Up applies migrations from migration dir.
func Up(c *config.Config) error {
	db, err := getDB(c)
	if err != nil {
		return err
	}

	defer closeDb(db)

	var insertTableQuery string

	var descriptionRegex *regexp.Regexp

	if c.Desc {
		descriptionRegex = regexp.MustCompile(`--\s*desc:\s*(.*)`)
		insertTableQuery = fmt.Sprintf("INSERT INTO %s (version, description) VALUES ($1, $2)", c.FullTableName)
	} else {
		insertTableQuery = fmt.Sprintf("INSERT INTO %s (version) VALUES ($1)", c.FullTableName)
	}

	migrations, err := getMigrationFiles(c.Dir)
	if err != nil {
		return err
	}

	if dupes := duplicateVersions(migrations); len(dupes) > 0 {
		return fmt.Errorf("duplicate migration versions detected:\n%s\nresolve manually: rename or remove one of the files",
			strings.Join(dupes, "\n"))
	}

	err = createTable(db, c)
	if err != nil {
		return err
	}

	slices.SortFunc(migrations, func(a fileVersion, b fileVersion) int {
		return cmp.Compare(a.version, b.version)
	})

	appliedVersions, err := appliedVersions(db, c)
	if err != nil {
		return err
	}

	for _, migration := range migrations {
		if _, ok := appliedVersions[migration.version]; ok {
			continue
		}

		err = applyMigration(db, c, migration, insertTableQuery, descriptionRegex)
		if err != nil {
			return err
		}
	}

	return nil
}

// applyMigration applies migration from file.
func applyMigration(db *sql.DB, c *config.Config, migration fileVersion,
	insertTableQuery string, descriptionRegex *regexp.Regexp,
) (err error) {
	content, err := os.ReadFile(migration.file)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("failed to begin tx: %w", err)
	}

	defer func() {
		rollbackErr := tx.Rollback()
		if errors.Is(rollbackErr, sql.ErrTxDone) {
			return
		}

		if rollbackErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to rollback tx: %w", rollbackErr))
		}
	}()

	_, err = tx.ExecContext(context.Background(), string(content))
	if err != nil {
		return fmt.Errorf("failed to execute migration file: %s err: %w", migration.file, err)
	}

	if c.Desc {
		var desc string

		match := descriptionRegex.FindSubmatch(content)
		if len(match) > 1 {
			desc = string(match[1])
		}

		if len(desc) == 0 {
			name := strings.TrimSuffix(filepath.Base(migration.file), ".sql")

			if _, after, found := strings.Cut(name, "_"); found {
				desc = after
			}
		}

		_, err = tx.ExecContext(context.Background(), insertTableQuery, migration.version, desc)
	} else {
		_, err = tx.ExecContext(context.Background(), insertTableQuery, migration.version)
	}

	if err != nil {
		return fmt.Errorf("failed to insert into table: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit tx: %w", err)
	}

	fmt.Printf("migrated %s\n", migration.file)

	return nil
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

// appliedVersions returns a map of applied versions migrations.
func appliedVersions(db *sql.DB, c *config.Config) (map[int64]struct{}, error) {
	//nolint:gosec
	sql := fmt.Sprintf("SELECT version FROM %s ORDER BY version", c.FullTableName)

	rows, err := db.QueryContext(context.Background(), sql)
	if err != nil {
		return nil, fmt.Errorf("failed to get applied versions: %w", err)
	}

	defer func() { _ = rows.Close() }()

	versions := make(map[int64]struct{})

	for rows.Next() {
		var version int64

		err = rows.Scan(&version)
		if err != nil {
			return nil, fmt.Errorf("failed to read version: %w", err)
		}

		versions[version] = struct{}{}
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("failed to read versions rows: %w", err)
	}

	return versions, nil
}

// closeDb closes the database connection.
func closeDb(db *sql.DB) {
	err := db.Close()
	if err != nil {
		log.Printf("failed to close db: %s\n", err)
	}
}

// createTable creates a migration table if not exists, or updates it.
func createTable(db *sql.DB, c *config.Config) (err error) {
	createTableQuery := `CREATE TABLE IF NOT EXISTS %s (%s)`

	params := []string{"version BIGINT PRIMARY KEY"}
	if c.Short {
		params[0] = "version INT PRIMARY KEY"
	}

	if c.Desc {
		params = append(params, "description TEXT")
	}

	if c.Ts {
		params = append(params, "applied_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP")
	}

	createTableQuery = fmt.Sprintf(createTableQuery, c.FullTableName, strings.Join(params, ", "))

	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("failed to begin tx of create table: %w", err)
	}

	defer func() {
		rollbackErr := tx.Rollback()
		if errors.Is(rollbackErr, sql.ErrTxDone) {
			return
		}

		if rollbackErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to rollback tx of create table: %w", rollbackErr))
		}
	}()

	_, err = tx.ExecContext(context.Background(), createTableQuery)
	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	if c.Desc {
		_, err := tx.ExecContext(context.Background(), fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS description TEXT", c.FullTableName))
		if err != nil {
			return fmt.Errorf("failed to add description column: %w", err)
		}
	}

	if c.Ts {
		_, err := tx.ExecContext(context.Background(), fmt.Sprintf(
			"ALTER TABLE %s ADD COLUMN IF NOT EXISTS applied_at TIMESTAMPTZ", c.FullTableName))
		if err != nil {
			return fmt.Errorf("failed to add applied_at column: %w", err)
		}

		_, err = tx.ExecContext(context.Background(), fmt.Sprintf(
			"ALTER TABLE %s ALTER COLUMN applied_at SET DEFAULT CURRENT_TIMESTAMP", c.FullTableName))
		if err != nil {
			return fmt.Errorf("failed to set default for applied_at: %w", err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit tx of create table: %w", err)
	}

	return nil
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
