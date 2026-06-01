package sessioncontext

import "context"

type key struct{}

func WithSessionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, key{}, id)
}

func SessionID(ctx context.Context) string {
	id, _ := ctx.Value(key{}).(string)
	return id
}
