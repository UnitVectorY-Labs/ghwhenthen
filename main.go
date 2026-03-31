package main

import (
	"runtime/debug"
)

var Version = "dev"

func main() {
	if Version == "dev" || Version == "" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			if bi.Main.Version != "" && bi.Main.Version != "(devel)" {
				Version = bi.Main.Version
			}
		}
	}

	// TODO: Implement everything in the internal package
}
