package claudedock

import (
	"context"
	"io"
	"sync/atomic"

	"golang.org/x/crypto/ssh"
)

// ColdPromoter performs background promotion of cold files to hot.
// Phase 37: Full implementation with inotify-based promotion is planned.
// Currently a minimal stub so mount_strategy.go compiles on all platforms.
type ColdPromoter struct {
	connB    *ssh.Client
	coldRoot string
	hotRoot  string
	logger   io.Writer
	pidFile  string

	promotionCount       atomic.Int64
	promotionBytes       atomic.Int64
	promotionFailedCount atomic.Int64
}

// NewColdPromoter creates a new ColdPromoter.
func NewColdPromoter(connB *ssh.Client, coldRoot, hotRoot string, logger io.Writer, pidFile string) *ColdPromoter {
	return &ColdPromoter{
		connB:    connB,
		coldRoot: coldRoot,
		hotRoot:  hotRoot,
		logger:   logger,
		pidFile:  pidFile,
	}
}

// Run starts the promotion loop.
// Stub: Phase 37 will implement inotify watcher + rsync promotion.
func (p *ColdPromoter) Run(ctx context.Context) {
	<-ctx.Done()
}

// Wait waits for the promoter to finish.
// Stub: Phase 37 will wait for the background goroutine to exit.
func (p *ColdPromoter) Wait() {}

// Stats returns promotion statistics.
func (p *ColdPromoter) Stats() (count int, bytes int64, failed int) {
	return int(p.promotionCount.Load()), p.promotionBytes.Load(), int(p.promotionFailedCount.Load())
}
