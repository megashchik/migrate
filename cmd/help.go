package cmd

import (
	"fmt"

	"github.com/megashchik/migrate/config"
)

const version = "0.1.0"

// Help displays the primary tool instructions.
func Help(c *config.Config) {
	if c.Extra {
		fmt.Print(`Advanced Features (Flags):
  -t        string   Metadata table name (default: "schema_migrations")
  -desc              Enable 'description' column support in the metadata table
  -env-url  string   Custom environment variable for connection (default: "DATABASE_URL")
  -short             Use INT4 instead of INT8 for the version column

Database Commands (Require -conn or DATABASE_URL):
  (default)          Run all pending migrations (up)
  list               List applied migrations from the database with descriptions
  last               Show the latest version number stored in the database

Local Commands (No connection required):
  new <name>         Create a new numeric-prefixed migration file
                     Usage: migrate new <name> [flags]
                     Flags for 'new':
                       -desc  string   Add description comment to the SQL file
                       -f     string   Numeric version format (default: T)

Supported Numeric Formats (-f):
  0                  Incremental: 000001, 000002 (Auto-padded to 6 digits)
  0000               Incremental: 0001, 0002 (Custom width by number of zeros)
  U                  Unix Epoch:  1736512272 (Total seconds)
  T                  Timestamp:   20260110193112 (YYYYMMDDHHMMSS)

Note: If the generated version is less than or equal to the last version in the
directory, it will be automatically incremented (last + 1) to ensure order.
`)

		return
	}

	fmt.Printf(`migrate — a lightweight tool for numeric PostgreSQL migrations.

Version: %s

Usage:
  migrate [flags] [command]

Flags:
  -conn     string   Connection string (Required, or set DATABASE_URL env)
  -dir      string   Migrations directory (default: "./migrations")
  -extra             Show advanced commands and numeric formatting options

Examples:
  migrate -conn "postgres://user:pass@localhost:5432/db"
  migrate -extra help
`, version)
}

// Version prints the current tool version.
func Version() {
	fmt.Printf("migrate version %s\n", version)
}
