package process

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// parentPIDs maps every process to its parent from one Toolhelp snapshot.
// gopsutil's PpidWithContext takes a full system snapshot per call, so asking
// it for each process made every listing O(n²): about a second of CPU on a
// typical desktop, repeated by every refresh tick.
func parentPIDs() map[int32]int32 {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	parents := make(map[int32]int32)
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		parents[int32(entry.ProcessID)] = int32(entry.ParentProcessID)
	}
	return parents
}
