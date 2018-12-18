// +build ignore

package main

import (
	"fmt"
	"os"
	"regexp"

	"github.com/tmthrgd/go-bindata"
)

func main() {
	files, err := bindata.FindFiles(".", &bindata.FindFilesOptions{
		Ignore: []*regexp.Regexp{
			regexp.MustCompile(`\.go$`),
		},
		Recursive: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}

	f, err := os.OpenFile("data.go", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
	defer f.Close()

	err = files.Generate(f, &bindata.GenerateOptions{
		Package: "html",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
