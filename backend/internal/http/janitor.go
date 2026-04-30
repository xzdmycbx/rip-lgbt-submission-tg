package http

import (
	"context"
	"log/slog"
	"time"

	"github.com/ripyc/rip-lgbt-submission-tg/internal/auth"
	"github.com/ripyc/rip-lgbt-submission-tg/internal/submission"
)

// startJanitor runs background cleanup of expired sessions, login links, and
// soft-deleted drafts. It returns a stop function the App lifecycle should call.
func startJanitor(logger *slog.Logger, authStore *auth.Store, drafts *submission.Store) func() {
	ctx, cancel := context.WithCancel(context.Background())
	go run(ctx, logger, authStore, drafts)
	return cancel
}

func run(ctx context.Context, logger *slog.Logger, authStore *auth.Store, drafts *submission.Store) {
	tick := time.NewTicker(6 * time.Hour)
	defer tick.Stop()
	doSweep(ctx, logger, authStore, drafts)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			doSweep(ctx, logger, authStore, drafts)
		}
	}
}

func doSweep(ctx context.Context, logger *slog.Logger, authStore *auth.Store, drafts *submission.Store) {
	if err := authStore.CleanupSessions(ctx); err != nil {
		logger.Warn("janitor: cleanup sessions", "err", err)
	}
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	if n, err := drafts.PurgeOlderThan(ctx, cutoff); err != nil {
		logger.Warn("janitor: purge drafts", "err", err)
	} else if n > 0 {
		logger.Info("janitor: purged drafts", "count", n)
	}
}
