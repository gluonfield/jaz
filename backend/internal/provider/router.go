package provider

import (
	"context"
	"fmt"
	"strings"
)

type Router struct {
	Default  string
	Provider map[string]Provider
}

func NewRouter(defaultProvider string, providers map[string]Provider) *Router {
	return &Router{
		Default:  strings.ToLower(strings.TrimSpace(defaultProvider)),
		Provider: providers,
	}
}

func (r *Router) Complete(ctx context.Context, req Request) (Response, error) {
	provider, err := r.provider(req.Provider)
	if err != nil {
		return Response{}, err
	}
	return provider.Complete(ctx, req)
}

func (r *Router) StreamComplete(ctx context.Context, req Request) (<-chan Event, error) {
	provider, err := r.provider(req.Provider)
	if err != nil {
		return nil, err
	}
	return provider.StreamComplete(ctx, req)
}

func (r *Router) provider(id string) (Provider, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		id = r.Default
	}
	if id == "" {
		return nil, fmt.Errorf("native provider is required")
	}
	provider, ok := r.Provider[id]
	if !ok || provider == nil {
		return nil, fmt.Errorf("native provider %q is not available", id)
	}
	return provider, nil
}

type UnavailableProvider struct {
	ID     string
	Reason string
}

func (p UnavailableProvider) Complete(context.Context, Request) (Response, error) {
	return Response{}, p.err()
}

func (p UnavailableProvider) StreamComplete(context.Context, Request) (<-chan Event, error) {
	return nil, p.err()
}

func (p UnavailableProvider) err() error {
	if strings.TrimSpace(p.Reason) != "" {
		return fmt.Errorf("native provider %q is not available: %s", p.ID, p.Reason)
	}
	return fmt.Errorf("native provider %q is not available", p.ID)
}
