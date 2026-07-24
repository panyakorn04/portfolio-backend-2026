package handler

import (
	"context"
	"sync"
	"time"

	"portfolio-backend/internal/observability"
	"portfolio-backend/internal/svc"
)

const (
	portfolioChatRetentionInterval  = time.Hour
	portfolioChatRetentionBatchSize = 100
	portfolioChatRetentionMaxPasses = 20
)

type PortfolioChatRetentionRunner struct {
	service *svc.ServiceContext
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func NewPortfolioChatRetentionRunner(service *svc.ServiceContext) *PortfolioChatRetentionRunner {
	return &PortfolioChatRetentionRunner{service: service}
}

func (runner *PortfolioChatRetentionRunner) Start(parent context.Context) {
	if runner == nil || runner.service == nil || runner.service.PortfolioChatSessions == nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	runner.cancel = cancel
	runner.wg.Add(1)
	go runner.loop(ctx)
}

func (runner *PortfolioChatRetentionRunner) Close() {
	if runner == nil || runner.cancel == nil {
		return
	}
	runner.cancel()
	runner.wg.Wait()
}

func (runner *PortfolioChatRetentionRunner) loop(ctx context.Context) {
	defer runner.wg.Done()
	runner.run(ctx)
	ticker := time.NewTicker(portfolioChatRetentionInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runner.run(ctx)
		}
	}
}

func (runner *PortfolioChatRetentionRunner) run(ctx context.Context) {
	maxMessages := portfolioChatMaxStoredMessages(runner.service)
	for pass := 0; pass < portfolioChatRetentionMaxPasses && ctx.Err() == nil; pass++ {
		result, err := runner.service.PortfolioChatSessions.PruneRetention(ctx, maxMessages, time.Now().UTC(), portfolioChatRetentionBatchSize)
		if err != nil {
			if ctx.Err() == nil {
				observability.Error(ctx, "portfolio_chat.retention.failed", "Portfolio chat retention failed", err)
			}
			return
		}
		if result.MessagesDeleted == 0 && result.SessionsDeleted == 0 {
			return
		}
	}
}
