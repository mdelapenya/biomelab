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

// RefreshManager manages periodic refresh cycles for a single repo.
type RefreshManager struct {
	repo            *git.Repository
	detector        *agent.Detector
	ideDetector     *ide.Detector
	termDetector    *terminal.Detector
	procLister      process.Lister
	prProv          provider.PRProvider
	cliAvail        provider.CLIAvailability
	networkInterval time.Duration

	mu            sync.Mutex
	ctx           context.Context
	cancel        context.CancelFunc
	sbxCandidates []string

	// OnRefresh is called with refresh results. The caller is responsible
	// for marshaling UI updates to the main thread (via fyne.Do).
	OnRefresh func(ops.RefreshResult)
}

// NewRefreshManager creates a refresh manager for the given repo.
func NewRefreshManager(
	repo *git.Repository,
	detector *agent.Detector,
	ideDetector *ide.Detector,
	termDetector *terminal.Detector,
	procLister process.Lister,
	prProv provider.PRProvider,
	networkInterval time.Duration,
) *RefreshManager {
	return &RefreshManager{
		repo:            repo,
		detector:        detector,
		ideDetector:     ideDetector,
		termDetector:    termDetector,
		procLister:      procLister,
		prProv:          prProv,
		networkInterval: networkInterval,
	}
}

// SetSandboxCandidates configures the ordered list of sandbox names to
// check during refresh. Pass nil/empty to disable the sandbox status check.
func (rm *RefreshManager) SetSandboxCandidates(candidates []string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.sbxCandidates = candidates
}

// SetCLIAvail updates the cached CLI availability.
func (rm *RefreshManager) SetCLIAvail(avail provider.CLIAvailability) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.cliAvail = avail
}

// Start begins the local and network refresh goroutines.
func (rm *RefreshManager) Start() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if rm.cancel != nil {
		return // already running
	}
	rm.ctx, rm.cancel = context.WithCancel(context.Background())

	// Initial quick refresh.
	go rm.doQuick()

	// Check CLI availability once.
	go rm.doCLICheck()

	// Start periodic tickers.
	go rm.runLocalLoop(rm.ctx)
	go rm.runNetworkLoop(rm.ctx)
}

// Pause stops the refresh goroutines. Call Resume to restart.
func (rm *RefreshManager) Pause() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if rm.cancel != nil {
		rm.cancel()
		rm.cancel = nil
		rm.ctx = nil
	}
}

// Resume restarts the refresh goroutines after a pause.
func (rm *RefreshManager) Resume() {
	rm.Pause() // ensure clean state
	rm.Start()
}

// Stop permanently shuts down the refresh manager.
func (rm *RefreshManager) Stop() {
	rm.Pause()
}

// TriggerQuick runs an immediate quick refresh (branch names only).
func (rm *RefreshManager) TriggerQuick() {
	go rm.doQuick()
}

// TriggerLocal runs an immediate local refresh.
func (rm *RefreshManager) TriggerLocal() {
	go rm.doLocal()
}

// TriggerNetwork runs an immediate network refresh.
func (rm *RefreshManager) TriggerNetwork() {
	go rm.doNetwork()
}

func (rm *RefreshManager) runLocalLoop(ctx context.Context) {
	ticker := time.NewTicker(localRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rm.doLocal()
		}
	}
}

func (rm *RefreshManager) runNetworkLoop(ctx context.Context) {
	// Run initial network refresh immediately.
	rm.doNetwork()

	ticker := time.NewTicker(rm.networkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rm.doNetwork()
		}
	}
}

func (rm *RefreshManager) doQuick() {
	result := ops.QuickRefresh(rm.repo)
	if rm.OnRefresh != nil {
		rm.OnRefresh(result)
	}
}

func (rm *RefreshManager) doLocal() {
	rm.mu.Lock()
	candidates := rm.sbxCandidates
	rm.mu.Unlock()

	result := ops.LocalRefresh(rm.repo, rm.detector, rm.ideDetector, rm.termDetector, rm.procLister, candidates)
	if rm.OnRefresh != nil {
		rm.OnRefresh(result)
	}
}

func (rm *RefreshManager) doNetwork() {
	rm.mu.Lock()
	candidates := rm.sbxCandidates
	cliAvail := rm.cliAvail
	rm.mu.Unlock()

	result := ops.NetworkRefresh(rm.repo, rm.detector, rm.ideDetector, rm.termDetector, rm.procLister, rm.prProv, cliAvail, candidates)
	if rm.OnRefresh != nil {
		rm.OnRefresh(result)
	}
}

func (rm *RefreshManager) doCLICheck() {
	if rm.prProv == nil {
		return
	}
	avail := ops.CheckCLI(rm.prProv)
	rm.SetCLIAvail(avail)
}
