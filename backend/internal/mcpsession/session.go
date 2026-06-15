package mcpsession

import (
	"context"
	"net/http"
	"strings"
)

const (
	HeaderName        = "X-Jaz-Session-ID"
	HeaderPlaceholder = "{{jaz_session_id}}"
)

type key struct{}

func With(ctx context.Context, id string) context.Context {
	id = strings.TrimSpace(id)
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, key{}, id)
}

func ID(ctx context.Context) string {
	id, _ := ctx.Value(key{}).(string)
	return strings.TrimSpace(id)
}

func Header(id string) http.Header {
	header := http.Header{}
	id = strings.TrimSpace(id)
	if id != "" {
		header.Set(HeaderName, id)
	}
	return header
}
