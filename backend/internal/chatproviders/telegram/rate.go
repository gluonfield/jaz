package telegram

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gotd/td/tgerr"
)

const (
	telegramAPIInterval            = 3 * time.Second
	telegramContactsSyncInterval   = 24 * time.Hour
	telegramBackfillMaxCallsPerRun = 30
	telegramBackfillRunInterval    = 30 * time.Minute
	telegramBackfillPageLimit      = 50
	telegramHistoricalWindow       = 365 * 24 * time.Hour
)

var errBackfillBudget = errors.New("telegram backfill budget exhausted")

type floodWaitError struct {
	wait time.Duration
}

func (e floodWaitError) Error() string {
	return fmt.Sprintf("telegram requested flood wait for %s", e.wait)
}

type backfillBudget struct {
	remaining int
}

func newBackfillBudget() *backfillBudget {
	return &backfillBudget{remaining: telegramBackfillMaxCallsPerRun}
}

func (b *backfillBudget) take() bool {
	if b.remaining <= 0 {
		return false
	}
	b.remaining--
	return true
}

func (p *Provider) waitAPI(ctx context.Context) error {
	p.apiMu.Lock()
	now := time.Now()
	wait := time.Duration(0)
	if now.Before(p.nextAPI) {
		wait = p.nextAPI.Sub(now)
		p.nextAPI = p.nextAPI.Add(telegramAPIInterval)
	} else {
		p.nextAPI = now.Add(telegramAPIInterval)
	}
	p.apiMu.Unlock()
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (p *Provider) backfillCall(ctx context.Context, budget *backfillBudget, fn func(context.Context) error) error {
	if !budget.take() {
		return errBackfillBudget
	}
	if err := p.waitAPI(ctx); err != nil {
		return err
	}
	err := fn(ctx)
	if wait, ok := tgerr.AsFloodWait(err); ok {
		return floodWaitError{wait: wait}
	}
	return err
}

func (p *Provider) foregroundCall(ctx context.Context, fn func(context.Context) error) error {
	if err := p.waitAPI(ctx); err != nil {
		return err
	}
	err := fn(ctx)
	if wait, ok := tgerr.AsFloodWait(err); ok {
		return floodWaitError{wait: wait}
	}
	return err
}
