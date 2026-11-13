package ingestion

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"
)

type WorkerConfig struct {
	PollInterval    time.Duration
	RetryDelay      time.Duration
	ProcessingLease time.Duration
	MaxAttempts     int
	Logger          *log.Logger
}

type Worker struct {
	service *Service
	config  WorkerConfig
	cancel  context.CancelFunc
	wake    chan struct{}
	wg      sync.WaitGroup
}

func NewWorker(service *Service, cfg WorkerConfig) (*Worker, error) {
	if service == nil || cfg.PollInterval <= 0 || cfg.RetryDelay <= 0 || cfg.ProcessingLease <= 0 || cfg.MaxAttempts <= 0 {
		return nil, errors.New("webhook worker service and positive configuration are required")
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}
	return &Worker{service: service, config: cfg, wake: make(chan struct{}, 1)}, nil
}

func (w *Worker) Start(parent context.Context) {
	if w == nil || w.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	w.wg.Add(1)
	go w.run(ctx)
}

func (w *Worker) Notify() {
	if w == nil {
		return
	}
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *Worker) Close() {
	if w == nil || w.cancel == nil {
		return
	}
	w.cancel()
	w.wg.Wait()
}

func (w *Worker) run(ctx context.Context) {
	defer w.wg.Done()
	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()
	for {
		w.drain(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-w.wake:
		}
	}
}

func (w *Worker) drain(ctx context.Context) {
	for ctx.Err() == nil {
		now := time.Now().UTC()
		claimed, err := w.service.ProcessNextWebhook(
			ctx, now.Add(-w.config.RetryDelay), now.Add(-w.config.ProcessingLease), w.config.MaxAttempts,
		)
		if err != nil {
			w.config.Logger.Printf("whatsapp webhook worker outcome=failed")
			return
		}
		if !claimed {
			return
		}
	}
}
