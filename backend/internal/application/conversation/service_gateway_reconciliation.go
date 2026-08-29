package conversation

import (
	"context"
	"time"

	"go.uber.org/zap"
)

const (
	gatewayDispatchReconcileEvery = 30 * time.Second
	gatewayDispatchGracePeriod    = 2 * time.Minute
	gatewayDispatchReconcileBatch = 100
)

func (s *Service) startGatewayDispatchReconciliationWorker(ctx context.Context) {
	if s == nil || s.repo == nil {
		return
	}
	go func() {
		s.reconcileOrphanGatewayTurns(ctx)
		ticker := time.NewTicker(gatewayDispatchReconcileEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.reconcileOrphanGatewayTurns(ctx)
			}
		}
	}()
}

func (s *Service) reconcileOrphanGatewayTurns(ctx context.Context) {
	for {
		reconcileCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		runIDs, err := s.repo.ReconcileOrphanGatewayTurns(
			reconcileCtx, time.Now().UTC().Add(-gatewayDispatchGracePeriod), gatewayDispatchReconcileBatch,
		)
		cancel()
		if err != nil {
			if ctx.Err() == nil && s.logger != nil {
				s.logger.Warn("gateway_dispatch_reconciliation_failed", zap.Error(err))
			}
			return
		}
		for _, runID := range runIDs {
			if err := s.publishMessageGenerationEventReliable(runID, map[string]interface{}{"type": "gateway_completed"}); err != nil && s.logger != nil {
				s.logger.Warn("gateway_dispatch_terminal_publish_failed", zap.String("run_id", runID), zap.Error(err))
			}
			s.generationStreams.finish(context.Background(), runID)
		}
		if len(runIDs) < gatewayDispatchReconcileBatch {
			break
		}
	}

	reconcileCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	candidates, err := s.repo.ListStaleGatewayRuns(
		reconcileCtx, time.Now().UTC().Add(-gatewayDispatchGracePeriod), gatewayDispatchReconcileBatch,
	)
	if err != nil {
		if ctx.Err() == nil && s.logger != nil {
			s.logger.Warn("gateway_generation_reconciliation_failed", zap.Error(err))
		}
		return
	}
	for _, candidate := range candidates {
		s.ReconcileInterruptedMessageGeneration(reconcileCtx, candidate.UserID, candidate.RunID)
	}
}
