package main

import (
	"fmt"
	"os"
	"time"
)

var version = "dev"

const defaultRefreshInterval = 30 * time.Second

// resolveRefreshInterval applies the precedence: CLI flag > BIOME_REFRESH env > default.
func resolveRefreshInterval(flagVal time.Duration) time.Duration {
	if flagVal != 0 {
		return flagVal
	}
	if val := os.Getenv("BIOME_REFRESH"); val != "" {
		d, err := time.ParseDuration(val)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid BIOME_REFRESH value %q: %v\n", val, err)
			os.Exit(1)
		}
		return d
	}
	return defaultRefreshInterval
}
