//go:generate go run bundle.go

package html

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Get(file string) ([]byte, error) {
	// If the file doesn't exist locally (We are running in a dev environment)
	if _, err := os.Stat(file); os.IsNotExist(err) {
		filePath := strings.TrimPrefix(filepath.ToSlash(
			strings.TrimPrefix(file, "/html")), "/")
		fmt.Printf("asset file: %s\n", file)
		return Asset(filePath)
	}

	// First attempt to load the file from disk
	fd, err := os.Open(file)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: file, Err: err}
	}
	defer fd.Close()

	buf := bytes.NewBuffer([]byte{})
	buf.ReadFrom(fd)

	return buf.Bytes(), nil
}
