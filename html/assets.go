//go:generate go run bundle.go

package html

import (
	"bytes"
	"os"
)

func Get(file string) ([]byte, error) {
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return Asset(file)
	}

	// First attempt to load the file from disk
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	buf := bytes.NewBuffer([]byte{})
	buf.ReadFrom(f)

	return buf.Bytes(), nil
}
