package cmd

import (
	"cmp"
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

	dimension := "BIGINT"
	if c.Short {
		dimension = "INT"
	}

	var createTableQuery string

	var insertTableQuery string

	var descriptionRegex *regexp.Regexp

	if c.Desc {
		descriptionRegex = regexp.MustCompile(`--\s*desc:\s*(.*)`)
		createTableQuery = fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (version %s PRIMARY KEY, description text)", c.FullTableName, dimension)
		insertTableQuery = fmt.Sprintf("INSERT INTO %s (version, description) VALUES ($1, $2)", c.FullTableName)
	} else {
		createTableQuery = fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (version %s PRIMARY KEY)", c.FullTableName, dimension)
		insertTableQuery = fmt.Sprintf("INSERT INTO %s (version) VALUES ($1)", c.FullTableName)
	}

	_, err = db.Exec(createTableQuery)
	if err != nil {
		return fmt.Errorf("failed to create table, err: %w", err)
	}

	files, err := filepath.Glob(c.Dir + "/*.sql")
	if err != nil {
		return fmt.Errorf("failed to get files: %w", err)
	}

	migrations := make([]fileVersion, 0, len(files))
	for _, f := range files {
		version, err := getVersion(f)
		if err != nil {
			return err
		}

		migrations = append(migrations, fileVersion{f, version})
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
func applyMigration(db *sql.DB, c *config.Config, migration fileVersion, insertTableQuery string, descriptionRegex *regexp.Regexp) error {
	content, err := os.ReadFile(migration.file)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin tx: %w", err)
	}

	defer func() {
		err := tx.Rollback()
		if errors.Is(err, sql.ErrTxDone) {
			return
		}

		if err != nil {
			log.Printf("failed to rollback tx: %s", err)
		}
	}()

	_, err = tx.Exec(string(content))
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
			base := filepath.Base(migration.file)
			name := strings.TrimSuffix(base, ".sql")

			parts := strings.SplitN(name, "_", 2)
			if len(parts) > 1 {
				desc = parts[1]
			}
		}

		_, err = tx.Exec(insertTableQuery, migration.version, desc)
	} else {
		_, err = tx.Exec(insertTableQuery, migration.version)
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

	err = db.Ping()
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

	rows, err := db.Query(sql)
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
