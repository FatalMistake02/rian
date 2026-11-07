package main

import (
	"fmt"
	"os"

	"rian/internal/app"
)

var version = "0.2.2"

func main() {
	if err := app.Run(os.Args[1:], os.Stdin, os.Stdout, version); err != nil {
		fmt.Fprintln(os.Stderr, "rian:", err)
		os.Exit(1)
	}
}
