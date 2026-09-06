package cmd

import (
	"context"
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

	query, values, err := getQuery(c)
	if err != nil {
		return err
	}

	err = db.QueryRowContext(context.Background(), query).Scan(values...)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		fmt.Println("no migrations applied yet")
		return nil
	case err != nil:
		return fmt.Errorf("failed to get last version: %w", err)
	}

	fmt.Println(formatRow(values))

	return nil
}
