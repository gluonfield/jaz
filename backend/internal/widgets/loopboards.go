package widgets

import "github.com/wins/jaz/backend/internal/loops"

// loopBoards forwards the loops MCP tool to the existing board operations on
// Service. It exists only to bridge the package boundary (loops can't import
// widgets) and translate widget types to loop-domain types — it holds no logic
// of its own.
type loopBoards struct{ svc *Service }

// LoopBoards exposes the board operations the loops MCP tool needs as a
// loops.BoardService, without leaking widget types across the boundary.
func (s *Service) LoopBoards() loops.BoardService {
	return loopBoards{svc: s}
}

func (b loopBoards) ListBoards() ([]loops.BoardSummary, error) {
	boards, err := b.svc.Repo.ListBoards()
	if err != nil {
		return nil, err
	}
	out := make([]loops.BoardSummary, 0, len(boards))
	for _, board := range boards {
		out = append(out, loops.BoardSummary{
			ID:        board.ID,
			Name:      board.Name,
			IsDefault: board.IsDefault,
		})
	}
	return out, nil
}

func (b loopBoards) ValidateBoardIDs(boardIDs []string) error {
	return b.svc.ValidateBoardIDs(boardIDs)
}

func (b loopBoards) AssignLoopBoards(loop loops.Loop, boardIDs []string) error {
	_, err := b.svc.AssignLoopBoards(loop, boardIDs)
	return err
}

func (b loopBoards) BoardsForLoop(loopID string) ([]string, error) {
	_, boards, found, err := b.svc.StateForLoop(loopID)
	if err != nil || !found {
		return nil, err
	}
	return boards, nil
}
