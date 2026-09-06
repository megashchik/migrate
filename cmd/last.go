package cmd

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

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

	err = db.QueryRow(query).Scan(values...)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		fmt.Println("no migrations applied yet")
		return nil
	case err != nil:
		return fmt.Errorf("failed to get last version: %w", err)
	}

	fmt.Println(values...)

	return nil
}

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
