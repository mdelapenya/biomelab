package process

import (
	"context"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// Lister abstracts process enumeration for testability.
type Lister interface {
	Processes(ctx context.Context) ([]Info, error)
}

// Info holds the data we need from each OS process.
type Info struct {
	PID     int32
	PPID    int32
	Name    string
	Cmdline string
	Cwd     string
	Status  string
	Created time.Time
}

// OSLister uses gopsutil to enumerate real processes.
type OSLister struct{}

func (o *OSLister) Processes(ctx context.Context) ([]Info, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, err
	}

	var result []Info
	for _, p := range procs {
		name, err := p.NameWithContext(ctx)
		if err != nil {
			continue
		}
		var cmdline string
		if cl, err := p.CmdlineWithContext(ctx); err == nil {
			cmdline = cl
		}
		var ppid int32
		if pp, err := p.PpidWithContext(ctx); err == nil {
			ppid = pp
		}
		result = append(result, Info{
			PID:     p.Pid,
			PPID:    ppid,
			Name:    name,
			Cmdline: cmdline,
		})
	}
	return result, nil
}

// Enrich fills in Cwd, Status, and Created for a process.
func Enrich(ctx context.Context, info *Info) {
	p, err := process.NewProcess(info.PID)
	if err != nil {
		return
	}
	if cwd, err := p.CwdWithContext(ctx); err == nil {
		info.Cwd = cwd
	}
	if statuses, err := p.StatusWithContext(ctx); err == nil && len(statuses) > 0 {
		info.Status = strings.Join(statuses, ",")
	}
	if createTime, err := p.CreateTimeWithContext(ctx); err == nil {
		info.Created = time.UnixMilli(createTime)
	}
	if ppid, err := p.PpidWithContext(ctx); err == nil {
		info.PPID = ppid
	}
}
