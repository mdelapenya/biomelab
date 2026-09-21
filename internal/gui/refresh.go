package gui

import (
	"context"
	"sync"
	"time"

	"github.com/mdelapenya/biomelab/internal/agent"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/ide"
	"github.com/mdelapenya/biomelab/internal/ops"
	"github.com/mdelapenya/biomelab/internal/process"
	"github.com/mdelapenya/biomelab/internal/provider"
	"github.com/mdelapenya/biomelab/internal/terminal"
)

const localRefreshInterval = 5 * time.Second

type refreshKind int

const (
	quickRefresh refreshKind = iota
	localRefresh
	networkRefresh
	refreshKinds
)

type refreshRun struct {
	ctx        context.Context
	cancel     context.CancelFunc
	generation uint64
}

type refreshInputs struct {
	candidates []string
	cliAvail   provider.CLIAvailability
}

type refreshJob struct {
	run    *refreshRun
	inputs refreshInputs
}

type refreshLane struct {
	running bool
	pending *refreshJob
}

// RefreshManager bounds work to one active and one pending request per kind,
// including across pause/resume. A slow network refresh does not block local work.
type RefreshManager struct {
	repo            *git.Repository
	detector        *agent.Detector
	ideDetector     *ide.Detector
	termDetector    *terminal.Detector
	procLister      process.Lister
	prProv          provider.PRProvider
	networkInterval time.Duration

	mu            sync.Mutex
	run           *refreshRun
	generation    uint64
	stopped       bool
	lanes         [refreshKinds]refreshLane
	sbxCandidates []string
	cliAvail      provider.CLIAvailability
	cliChecked    bool

	// work/checkCLI are fixed before Start. Tests supply controllable operations.
	work     func(context.Context, refreshKind, refreshInputs) ops.RefreshResult
	checkCLI func(context.Context) provider.CLIAvailability

	// OnRefresh is fixed before Start. The UI must check IsCurrent(generation)
	// inside fyne.Do as an already queued result can outlive its run.
	OnRefresh func(ops.RefreshResult, uint64)
}

func NewRefreshManager(repo *git.Repository, detector *agent.Detector,
	ideDetector *ide.Detector, termDetector *terminal.Detector,
	procLister process.Lister, prProv provider.PRProvider, networkInterval time.Duration,
) *RefreshManager {
	rm := &RefreshManager{
		repo: repo, detector: detector, ideDetector: ideDetector,
		termDetector: termDetector, procLister: procLister, prProv: prProv,
		networkInterval: networkInterval, cliAvail: provider.CLINotFound,
	}
	rm.work = rm.perform
	if prProv != nil {
		rm.checkCLI = prProv.CheckCLIContext
	}
	return rm
}

func (rm *RefreshManager) SetSandboxCandidates(candidates []string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.sbxCandidates = append([]string(nil), candidates...)
}

func (rm *RefreshManager) SetCLIAvail(avail provider.CLIAvailability) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.cliAvail, rm.cliChecked = avail, true
}

func (rm *RefreshManager) Start() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if rm.stopped || rm.run != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	rm.generation++
	run := &refreshRun{ctx: ctx, cancel: cancel, generation: rm.generation}
	rm.run = run
	rm.enqueueLocked(quickRefresh, run)
	rm.enqueueLocked(networkRefresh, run)
	go rm.loop(run, localRefresh, localRefreshInterval)
	go rm.loop(run, networkRefresh, rm.networkInterval)
}

func (rm *RefreshManager) pauseLocked() {
	if rm.run != nil {
		rm.run.cancel()
		rm.run = nil
	}
	for i := range rm.lanes {
		rm.lanes[i].pending = nil
	}
}

// Pause cancels current work and discards pending requests and late results.
func (rm *RefreshManager) Pause() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.pauseLocked()
}

func (rm *RefreshManager) Resume() { rm.Pause(); rm.Start() }

// Stop is terminal: later triggers or Resume cannot restart this manager.
func (rm *RefreshManager) Stop() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.stopped = true
	rm.pauseLocked()
}

func (rm *RefreshManager) IsCurrent(generation uint64) bool {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	return rm.run != nil && rm.run.generation == generation && rm.run.ctx.Err() == nil
}

func (rm *RefreshManager) TriggerQuick() { rm.trigger(quickRefresh) }
func (rm *RefreshManager) TriggerLocal() { rm.trigger(localRefresh) }

// TriggerNetwork is a deliberate refresh, so it also rechecks CLI availability
// after the user has installed or authenticated a tool. Ticker refreshes reuse the cached result.
func (rm *RefreshManager) TriggerNetwork() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if rm.run != nil {
		rm.cliChecked = false
		rm.enqueueLocked(networkRefresh, rm.run)
	}
}

func (rm *RefreshManager) trigger(kind refreshKind) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if rm.run != nil {
		rm.enqueueLocked(kind, rm.run)
	}
}

func (rm *RefreshManager) enqueueLocked(kind refreshKind, run *refreshRun) {
	if rm.run != run || run.ctx.Err() != nil {
		return
	}
	lane := &rm.lanes[kind]
	// Replacing the single pending request coalesces bursts using the latest inputs.
	lane.pending = &refreshJob{run, refreshInputs{append([]string(nil), rm.sbxCandidates...), rm.cliAvail}}
	if !lane.running {
		lane.running = true
		go rm.drain(kind)
	}
}

func (rm *RefreshManager) loop(run *refreshRun, kind refreshKind, interval time.Duration) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-run.ctx.Done():
			return
		case <-ticker.C:
			rm.mu.Lock()
			rm.enqueueLocked(kind, run)
			rm.mu.Unlock()
		}
	}
}

func (rm *RefreshManager) drain(kind refreshKind) {
	for {
		rm.mu.Lock()
		lane := &rm.lanes[kind]
		job := lane.pending
		lane.pending = nil
		if job == nil {
			lane.running = false
			rm.mu.Unlock()
			return
		}
		rm.mu.Unlock()
		if job.run.ctx.Err() != nil {
			continue
		}

		if kind == networkRefresh {
			rm.mu.Lock()
			checked := rm.cliChecked
			rm.mu.Unlock()
			if !checked && rm.checkCLI != nil {
				avail := rm.checkCLI(job.run.ctx)
				rm.mu.Lock()
				if rm.run == job.run && job.run.ctx.Err() == nil {
					rm.cliAvail, rm.cliChecked = avail, true
				}
				rm.mu.Unlock()
			}
			rm.mu.Lock()
			job.inputs.cliAvail = rm.cliAvail
			rm.mu.Unlock()
		}
		if job.run.ctx.Err() != nil {
			continue
		}
		result := rm.work(job.run.ctx, kind, job.inputs)
		if rm.OnRefresh != nil && rm.IsCurrent(job.run.generation) {
			rm.OnRefresh(result, job.run.generation)
		}
	}
}

func (rm *RefreshManager) perform(ctx context.Context, kind refreshKind, inputs refreshInputs) ops.RefreshResult {
	switch kind {
	case quickRefresh:
		return ops.QuickRefresh(rm.repo)
	case localRefresh:
		return ops.LocalRefresh(ctx, rm.repo, rm.detector, rm.ideDetector, rm.termDetector, rm.procLister, inputs.candidates)
	default:
		return ops.NetworkRefresh(ctx, rm.repo, rm.detector, rm.ideDetector, rm.termDetector, rm.procLister, rm.prProv, inputs.cliAvail, inputs.candidates)
	}
}
