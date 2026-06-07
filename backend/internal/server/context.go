package server

import (
	"context"
	"time"
)

const serverActionTimeout = 30 * time.Second

func serverActionContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), serverActionTimeout)
}
