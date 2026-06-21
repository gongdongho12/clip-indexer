package main

import (
	"fmt"
	"os"

	"clip-indexer/internal/media"
)

func main() {
	if err := media.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "clip-indexer: %v\n", err)
		os.Exit(1)
	}
}
