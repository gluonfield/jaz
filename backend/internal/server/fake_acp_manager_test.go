package server

import (
	"context"
	"sync"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/storage"
)

type fakeACPManager struct {
	mu             sync.Mutex
	sent           acp.SendRequest
	continueGoals  []string
	compacted      acp.CompactRequest
	steered        acp.SteerRequest
	sideChat       acp.SideChatRequest
	answered       acp.InteractiveAnswer
	sendCtxErr     error
	compactCtxErr  error
	steerCtxErr    error
	sendPlatform   string
	sideCtxErr     error
	answerCtxErr   error
	cancelCtxErr   error
	cancelled      bool
	sendErr        error
	compactErr     error
	steerErr       error
	steerSupported bool
	sideErr        error
	answerErr      error
	job            acp.Job
	jobs           []acp.Job
	spawnStore     storage.SessionStore
	spawned        acp.SpawnRequest
	created        acp.SpawnRequest
	spawnErr       error
	utilityPrompt  acp.UtilityPromptRequest
	utilityText    string
	utilityErr     error
	cancelRelease  chan struct{}
}

func (f *fakeACPManager) CreateSession(_ context.Context, req acp.SpawnRequest) (storage.Session, error) {
	f.mu.Lock()
	f.created = req
	spawnStore := f.spawnStore
	spawnErr := f.spawnErr
	f.mu.Unlock()
	if spawnErr != nil {
		return storage.Session{}, spawnErr
	}
	if spawnStore == nil {
		return storage.Session{}, nil
	}
	return spawnStore.CreateSession(storage.CreateSession{
		Slug:            req.Slug,
		Title:           req.Title,
		Runtime:         storage.RuntimeACP,
		ModelProvider:   req.ACPAgent,
		Model:           req.Model,
		ReasoningEffort: req.ReasoningEffort,
		SourceType:      req.SourceType,
		SourceID:        req.SourceID,
		RuntimeRef: &storage.RuntimeRef{
			Type:            storage.RuntimeACP,
			Agent:           req.ACPAgent,
			Cwd:             req.Directory,
			ArtifactSurface: req.ArtifactSurface,
			MCPServerPolicy: req.MCPServerPolicy,
		},
	})
}

func (f *fakeACPManager) Spawn(_ context.Context, req acp.SpawnRequest) (acp.SpawnResult, error) {
	f.mu.Lock()
	f.spawned = req
	spawnStore := f.spawnStore
	spawnErr := f.spawnErr
	f.mu.Unlock()
	if spawnErr != nil {
		return acp.SpawnResult{}, spawnErr
	}
	if spawnStore == nil {
		return acp.SpawnResult{}, nil
	}
	session, err := spawnStore.CreateSession(storage.CreateSession{
		Slug:       req.Slug,
		Title:      req.Title,
		Runtime:    storage.RuntimeACP,
		SourceType: req.SourceType,
		SourceID:   req.SourceID,
		RuntimeRef: &storage.RuntimeRef{
			Type:            storage.RuntimeACP,
			Agent:           req.ACPAgent,
			SessionID:       "fake-acp-session",
			ArtifactSurface: req.ArtifactSurface,
			MCPServerPolicy: req.MCPServerPolicy,
		},
	})
	if err != nil {
		return acp.SpawnResult{}, err
	}
	return acp.SpawnResult{
		Status:    "created",
		SessionID: session.ID,
		Slug:      session.Slug,
		ACPAgent:  req.ACPAgent,
		State:     acp.StateIdle,
		Session:   session,
	}, nil
}

func (f *fakeACPManager) Agents() []string { return nil }

func (f *fakeACPManager) RunUtilityPrompt(_ context.Context, req acp.UtilityPromptRequest) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.utilityPrompt = req
	return f.utilityText, f.utilityErr
}

func (f *fakeACPManager) Send(ctx context.Context, req acp.SendRequest) (acp.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = req
	f.sendCtxErr = ctx.Err()
	f.sendPlatform = sessioncontext.ClientPlatform(ctx)
	return f.job, f.sendErr
}

func (f *fakeACPManager) ContinueGoal(_ context.Context, session string) (acp.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.continueGoals = append(f.continueGoals, session)
	return f.job, f.sendErr
}

func (f *fakeACPManager) Compact(ctx context.Context, req acp.CompactRequest) (acp.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.compacted = req
	f.compactCtxErr = ctx.Err()
	return f.job, f.compactErr
}

func (f *fakeACPManager) Steer(ctx context.Context, req acp.SteerRequest) (acp.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.steerSupported {
		return f.job, acp.ErrPromptQueueingUnsupported
	}
	f.steered = req
	f.steerCtxErr = ctx.Err()
	return f.job, f.steerErr
}

func (f *fakeACPManager) SendSideChat(ctx context.Context, req acp.SideChatRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sideChat = req
	f.sideCtxErr = ctx.Err()
	return f.sideErr
}

func (f *fakeACPManager) Status(string) (acp.Job, error) {
	return f.job, nil
}

func (f *fakeACPManager) List() []acp.Job {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.jobs != nil {
		return append([]acp.Job(nil), f.jobs...)
	}
	if f.job.ID == "" && f.job.Slug == "" {
		return nil
	}
	return []acp.Job{f.job}
}

func (f *fakeACPManager) AnswerInteractive(ctx context.Context, answer acp.InteractiveAnswer) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answered = answer
	f.answerCtxErr = ctx.Err()
	return f.answerErr
}

func (f *fakeACPManager) Cancel(ctx context.Context, _ string) (acp.Job, error) {
	f.mu.Lock()
	f.cancelled = true
	f.cancelCtxErr = ctx.Err()
	job := f.job
	release := f.cancelRelease
	f.mu.Unlock()
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
		}
	}
	return job, nil
}
