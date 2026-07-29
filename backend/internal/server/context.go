package server

import (
	"context"
	"time"
)

const (
	serverActionTimeout       = 30 * time.Second
	serverACPBootstrapTimeout = 5 * time.Minute
	serverSideChatTimeout     = 10 * time.Minute
)

func serverActionContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), serverActionTimeout)
}

func serverActionContextFrom(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), serverActionTimeout)
}

func serverACPBootstrapContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), serverACPBootstrapTimeout)
}

func serverACPBootstrapContextFrom(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), serverACPBootstrapTimeout)
}

func serverSideChatContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), serverSideChatTimeout)
}
