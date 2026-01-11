package cmd

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/megashchik/migrate/config"
)

// New creates a new migration file.
func New(c *config.Config) error {
	prefix, err := generateVersionPrefix(c)
	if err != nil {
		return err
	}

	var postfix string
	if len(c.CommandArg) != 0 {
		postfix = "_" + c.CommandArg
	}

	filename := filepath.Join(c.Dir, prefix+postfix+".sql")

	err = os.MkdirAll(c.Dir, 0o755)
	if err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	f, err := os.OpenFile(filename, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	switch {
	case errors.Is(err, os.ErrExist):
		return fmt.Errorf("file already exists: %s", filename)
	case err != nil:
		return fmt.Errorf("failed to create file: %w", err)
	}

	if c.Desc {
		_, err = fmt.Fprintf(f, "-- desc: %s\n", c.CommandArg)
		if err != nil {
			err = fmt.Errorf("failed to write to file, err: %w", err)
		}
	}

	closeErr := f.Close()
	if closeErr != nil {
		err = errors.Join(err, fmt.Errorf("failed to close file: %w", closeErr))
	}

	return err
}

// parseTimeVersion returns a version based on the current time.
func parseTimeVersion() (int64, error) {
	const defaultTimeFormat = "20060102150405"

	version, err := strconv.ParseInt(time.Now().Format(defaultTimeFormat), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse time, err: %w", err)
	}

	return version, nil
}

// generateVersionPrefix generates a version prefix for a new migration file.
func generateVersionPrefix(c *config.Config) (string, error) {
	f := c.Format

	lastVersion, err := getLastVersion(c.Dir)
	if err != nil {
		return "", err
	}

	var newVersion int64

	var padding int

	switch {
	case f == "U":
		newVersion = time.Now().Unix()
	case f == "T" || len(f) == 0:
		newVersion, err = parseTimeVersion()
		if err != nil {
			return "", err
		}
	case f == "0":
		padding = 6
		newVersion = lastVersion + 1
	case len(strings.TrimLeft(f, "0")) == 0:
		padding = len(f)
		newVersion = lastVersion + 1
	default:
		log.Println("unexpected format, used default")

		newVersion, err = parseTimeVersion()
		if err != nil {
			return "", err
		}
	}

	if newVersion <= lastVersion {
		log.Println("version overflow, will be increased")
		newVersion = lastVersion + 1
	}

	if padding > 0 {
		result := fmt.Sprintf("%0*d", padding, newVersion)
		if len(result) > padding {
			log.Println("overflow of padding, padding will be increased")
		}

		return result, nil
	}

	return strconv.FormatInt(newVersion, 10), nil
}

// getLastVersion returns the last migration version in the directory.
func getLastVersion(dir string) (int64, error) {
	_, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return 0, nil
	}

	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil {
		return 0, fmt.Errorf("failed to get files: %w", err)
	}

	if len(files) == 0 {
		return 0, nil
	}

	var maxVersion int64 = 0

	for _, file := range files {
		version, err := getVersion(file)
		if err != nil {
			return 0, err
		}

		if version > maxVersion {
			maxVersion = version
		}
	}

	return maxVersion, nil
}

// getVersion returns the version of a migration file.
func getVersion(filename string) (int64, error) {
	base := filepath.Base(filename)
	nameWithoutExt := strings.TrimSuffix(base, ".sql")

	index := strings.Index(nameWithoutExt, "_")
	if index == -1 {
		index = len(nameWithoutExt)
	}

	version, err := strconv.ParseInt(nameWithoutExt[:index], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("can't get version from filename %s: %w", filename, err)
	}

	return version, nil
}
