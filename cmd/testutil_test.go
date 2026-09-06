package cmd

import (
	"bytes"
	"io"
	"os"
)

// captureStdout runs f while capturing everything it writes to os.Stdout.
// It is panic-safe: os.Stdout is restored before the panic is re-raised.
func captureStdout(f func()) (string, error) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = w

	defer func() {
		os.Stdout = old
		_ = w.Close()
		if p := recover(); p != nil {
			panic(p)
		}
	}()

	f()

	if err := w.Close(); err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return "", err
	}

	return buf.String(), nil
}
