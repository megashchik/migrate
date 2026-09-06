package cmd

import (
	"fmt"

	"github.com/megashchik/migrate/config"
)

// Check verifies there are no duplicate migration versions in the directory.
// It is a local command and can be used as a CI gate.
func Check(c *config.Config) error {
	migrations, err := getMigrationFiles(c.Dir)
	if err != nil {
		return err
	}

	dupes := duplicateVersions(migrations)
	if len(dupes) == 0 {
		fmt.Println("ok: no duplicate migration versions")

		return nil
	}

	fmt.Println("duplicate migration versions detected:")
	for _, d := range dupes {
		fmt.Printf("  %s\n", d)
	}

	return fmt.Errorf("found %d duplicate version group(s), resolve manually", len(dupes))
}