package cmd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

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
