package cmd

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/megashchik/migrate/config"
)

// List prints a list of applied migrations.
func List(c *config.Config) error {
	db, err := getDB(c)
	if err != nil {
		return err
	}

	defer closeDb(db)

	hasDescription, err := hasDescriptionColumn(db, c)
	if err != nil {
		return err
	}

	query := "SELECT version, '' as description FROM %s ORDER BY version"
	if hasDescription {
		query = "SELECT version, description FROM %s ORDER BY version"
	}

	query = fmt.Sprintf(query, c.FullTableName)

	rows, err := db.Query(query)
	if err != nil {
		return fmt.Errorf("failed db query, err: %w", err)
	}

	defer func() { _ = rows.Close() }()

	found := false

	for rows.Next() {
		var version int64

		var description string

		err = rows.Scan(&version, &description)
		if err != nil {
			return fmt.Errorf("failed to read row, err: %w", err)
		}

		printVersion(hasDescription, version, description)

		found = true
	}

	err = rows.Err()
	if err != nil {
		return fmt.Errorf("failed to read rows, err: %w", err)
	}

	if !found {
		fmt.Println("no migrations applied yet")
	}

	return nil
}

// hasDescriptionColumn returns true if table has description column.
func hasDescriptionColumn(db *sql.DB, c *config.Config) (bool, error) {
	var hasDescription bool

	query := `SELECT EXISTS (
    SELECT 1 FROM information_schema.columns 
    WHERE table_schema=$1 AND (table_name=$2 or table_name=LOWER($2)) AND column_name='description'
)`

	err := db.QueryRow(query, strings.Trim(c.Schema, `"`), strings.Trim(c.Table, `"`)).Scan(&hasDescription)
	if err != nil {
		return false, fmt.Errorf("failed to check table, err: %w", err)
	}

	return hasDescription, nil
}
