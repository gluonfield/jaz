package provider

import (
	"context"
	"testing"
)

type namedProvider struct {
	name     string
	requests []Request
}

func (p *namedProvider) Complete(_ context.Context, req Request) (Response, error) {
	p.requests = append(p.requests, req)
	return Response{Message: AssistantMessage(p.name, nil)}, nil
}

func (p *namedProvider) StreamComplete(_ context.Context, req Request) (<-chan Event, error) {
	p.requests = append(p.requests, req)
	ch := make(chan Event, 1)
	close(ch)
	return ch, nil
}

func TestRouterDispatchesByRequestProvider(t *testing.T) {
	openRouter := &namedProvider{name: "openrouter"}
	openAI := &namedProvider{name: "openai"}
	router := NewRouter(ProviderOpenRouter, map[string]Provider{
		ProviderOpenRouter: openRouter,
		ProviderOpenAI:     openAI,
	})

	if _, err := router.Complete(context.Background(), Request{Provider: ProviderOpenAI, Model: "gpt-test"}); err != nil {
		t.Fatal(err)
	}
	if len(openAI.requests) != 1 || len(openRouter.requests) != 0 {
		t.Fatalf("request was not routed to openai: openai=%d openrouter=%d", len(openAI.requests), len(openRouter.requests))
	}

	if _, err := router.Complete(context.Background(), Request{Model: "default-test"}); err != nil {
		t.Fatal(err)
	}
	if len(openRouter.requests) != 1 {
		t.Fatalf("default request was not routed to openrouter: %d", len(openRouter.requests))
	}
}
