package main

import (
	"fmt"
	"os"

	"sshc/internal/bootstrap"
)

func main() {
	if err := bootstrap.NewApp().RunWithArgs(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}
