/*
Copyright © 2023 Thomas Güttler <info@thomas-guettler.de>
See LICENSE file.
*/
package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/gavv/cobradoc"
	"github.com/guettli/watchall/cmd"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "gendocs" {
		b := &bytes.Buffer{}
		err := cobradoc.WriteDocument(b, cmd.RootCmd, cobradoc.Markdown, cobradoc.Options{})
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		usageFile := "usage.md"
		err = os.WriteFile(usageFile, b.Bytes(), 0o600)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Printf("Created %q\n", usageFile)
		os.Exit(0)
	}

	cmd.Execute()
}
