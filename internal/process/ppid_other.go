//go:build !windows

package process

// parentPIDs returns nil: gopsutil reads a parent PID cheaply per process here.
func parentPIDs() map[int32]int32 { return nil }
