package cmd

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/megashchik/migrate/config"
)

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
		return fmt.Errorf("failed to begin transaction for migration table %s: %w", c.FullTableName, err)
	}

	defer func() {
		rollbackErr := tx.Rollback()
		if errors.Is(rollbackErr, sql.ErrTxDone) {
			return
		}

		if rollbackErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to rollback transaction for migration table %s: %w", c.FullTableName, rollbackErr))
		}
	}()

	_, err = tx.ExecContext(context.Background(), createTableQuery)
	if err != nil {
		return fmt.Errorf("failed to create migration table %s: %w", c.FullTableName, err)
	}

	if c.Desc {
		_, err := tx.ExecContext(context.Background(), fmt.Sprintf("ALTER TABLE %s ADD COLUMN IF NOT EXISTS description TEXT", c.FullTableName))
		if err != nil {
			return fmt.Errorf("failed to add description column to migration table %s: %w", c.FullTableName, err)
		}
	}

	if c.Ts {
		_, err := tx.ExecContext(context.Background(), fmt.Sprintf(
			"ALTER TABLE %s ADD COLUMN IF NOT EXISTS applied_at TIMESTAMPTZ", c.FullTableName))
		if err != nil {
			return fmt.Errorf("failed to add applied_at column to migration table %s: %w", c.FullTableName, err)
		}

		_, err = tx.ExecContext(context.Background(), fmt.Sprintf(
			"ALTER TABLE %s ALTER COLUMN applied_at SET DEFAULT CURRENT_TIMESTAMP", c.FullTableName))
		if err != nil {
			return fmt.Errorf("failed to set default for applied_at in migration table %s: %w", c.FullTableName, err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("failed to commit transaction for migration table %s: %w", c.FullTableName, err)
	}

	return nil
}
