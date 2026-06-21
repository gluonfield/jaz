package server

import (
	"context"
	"time"
)

const serverActionTimeout = 30 * time.Second

func serverActionContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), serverActionTimeout)
}

func serverActionContextFrom(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), serverActionTimeout)
}
