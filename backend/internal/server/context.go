package server

import (
	"context"
	"time"
)

const (
	serverActionTimeout   = 30 * time.Second
	serverSideChatTimeout = 10 * time.Minute
)

func serverActionContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), serverActionTimeout)
}

func serverSideChatContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), serverSideChatTimeout)
}
