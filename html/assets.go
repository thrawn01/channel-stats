//go:generate go run bundle.go

package html

import (
	"bytes"
	"os"
	"path"
)

func Get(file string) ([]byte, error) {
	filePath := path.Join("./html", file)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return Asset(file)
	}

	// First attempt to load the file from disk
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	buf := bytes.NewBuffer([]byte{})
	buf.ReadFrom(f)

	return buf.Bytes(), nil
}
