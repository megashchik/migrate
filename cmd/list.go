package cmd

import (
	"context"
	"fmt"

	"github.com/megashchik/migrate/config"
)

// List prints a list of applied migrations.
func List(c *config.Config) error {
	db, err := getDB(c)
	if err != nil {
		return err
	}

	defer closeDb(db)

	query, values, err := getQuery(c)
	if err != nil {
		return err
	}

	rows, err := db.QueryContext(context.Background(), query)
	if err != nil {
		return fmt.Errorf("failed db query, err: %w", err)
	}

	defer func() { _ = rows.Close() }()

	found := false

	for rows.Next() {
		err = rows.Scan(values...)
		if err != nil {
			return fmt.Errorf("failed to read row, err: %w", err)
		}

		fmt.Println(formatRow(values))

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
