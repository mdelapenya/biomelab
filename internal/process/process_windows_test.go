package process

import (
	"context"
	"os"
	"testing"
)

func TestParentPIDsIncludesThisProcess(t *testing.T) {
	parents := parentPIDs()
	if got := parents[int32(os.Getpid())]; got != int32(os.Getppid()) {
		t.Fatalf("parent of %d = %d, want %d", os.Getpid(), got, os.Getppid())
	}
}

func TestOSListerReportsParents(t *testing.T) {
	procs, err := (&OSLister{}).Processes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range procs {
		if p.PID == int32(os.Getpid()) {
			if p.PPID != int32(os.Getppid()) {
				t.Fatalf("PPID = %d, want %d", p.PPID, os.Getppid())
			}
			return
		}
	}
	t.Fatal("this process was not listed")
}

func BenchmarkOSListerProcesses(b *testing.B) {
	l := &OSLister{}
	for i := 0; i < b.N; i++ {
		if _, err := l.Processes(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}
