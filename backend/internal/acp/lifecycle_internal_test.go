package acp

import (
	"errors"
	"io"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/gluonfield/acp-transport/jsonrpc"
)

func TestWaitServeErrRecordsSynchronousTransportClosure(t *testing.T) {
	peer := new(jsonrpc.Peer)
	process := &agentProcess{peer: peer, serveDone: make(chan struct{})}
	job := &jobState{Job: Job{ID: "thread", State: StateRunning}}
	manager := &Manager{
		log:       log.New(io.Discard),
		jobsByID:  map[string]*jobState{"thread": job},
		processes: map[string]*agentProcess{"thread": process},
	}

	err := manager.waitServeErr(peer, jsonrpc.ErrClosed)
	if !errors.Is(err, jsonrpc.ErrClosed) {
		t.Fatalf("error = %v, want connection closed", err)
	}
	if got := manager.serveErr("thread"); !errors.Is(got, jsonrpc.ErrClosed) {
		t.Fatalf("stored error = %v, want connection closed", got)
	}
	select {
	case <-process.serveDone:
	default:
		t.Fatal("serve failure signal is still open")
	}
}
