package boards

import (
	"context"

	"github.com/wins/jaz/backend/internal/loops"
)

// loopBoards adapts the service to loops.BoardService, which loops owns
// because it cannot import this package.
type loopBoards struct{ svc *Service }

func (s *Service) LoopBoards() loops.BoardService {
	return loopBoards{svc: s}
}

func (b loopBoards) ListBoards() ([]loops.BoardSummary, error) {
	boards, err := b.svc.ListBoards(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]loops.BoardSummary, 0, len(boards))
	for _, board := range boards {
		out = append(out, loops.BoardSummary{ID: board.ID, Name: board.Title})
	}
	return out, nil
}

func (b loopBoards) ValidateBoardIDs(boardIDs []string) error {
	return b.svc.ValidateBoardIDs(context.Background(), boardIDs)
}

func (b loopBoards) AssignLoopBoards(loop loops.Loop, boardIDs []string) error {
	return b.svc.AssignLoopBoards(context.Background(), loop, boardIDs)
}

func (b loopBoards) BoardsForLoop(loopID string) ([]string, error) {
	_, boardIDs, err := b.svc.StateForLoop(context.Background(), loopID)
	return boardIDs, err
}
