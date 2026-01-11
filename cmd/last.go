package cmd

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/megashchik/migrate/config"
)

// Last prints the last applied migration.
func Last(c *config.Config) error {
	db, err := getDB(c)
	if err != nil {
		return err
	}

	defer closeDb(db)

	hasDescription, err := hasDescriptionColumn(db, c)
	if err != nil {
		return err
	}

	query := "SELECT version, '' as description FROM %s ORDER BY version DESC LIMIT 1"
	if hasDescription {
		query = "SELECT version, description FROM %s ORDER BY version DESC LIMIT 1"
	}

	query = fmt.Sprintf(query, c.FullTableName)

	var version int64

	var description string

	err = db.QueryRow(query).Scan(&version, &description)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		fmt.Println("no migrations applied yet")
		return nil
	case err != nil:
		return fmt.Errorf("failed to get last version: %w", err)
	}

	printVersion(hasDescription, version, description)

	return nil
}

// printVersion prints the migration version and description.
func printVersion(hasDescription bool, version int64, description string) {
	if hasDescription {
		fmt.Printf("%d: %s\n", version, description)
		return
	}

	fmt.Println(version)
}
