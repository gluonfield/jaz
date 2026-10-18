package loops

import (
	"strings"
	"time"
)

// awayAfter is how long the user may go without writing before bots'
// routines hold their runs.
const awayAfter = 72 * time.Hour

const awayReason = "Skipped: you had not written to Jaz for 3 days."

// GuardAway holds the runs of every bot's routine that does not keep running
// once the user has written nothing for awayAfter, and lets them run again
// with the user's next message. A routine's own status never changes. Each
// bot tells the user when its routines hold and when they go on; the notice
// is best-effort and never repeated, since the stored marker, not the
// notice, is what holds the routines.
func (s *Service) GuardAway() error {
	last, err := s.Repo.LastUserMessageAt()
	if err != nil {
		return err
	}
	since, err := s.Repo.AwaySince()
	if err != nil {
		return err
	}
	held := !since.IsZero()
	s.mu.Lock()
	s.away = !last.IsZero() && s.now().Sub(last) >= awayAfter || held && !last.After(since)
	away := s.away
	s.mu.Unlock()
	if away == held {
		return nil
	}
	since = time.Time{}
	if away {
		since = last
	}
	if err := s.Repo.SetAwaySince(since); err != nil {
		return err
	}
	s.tellAway(away)
	return nil
}

// HeldBots are the bots with routines whose runs wait for the user to come
// back; nil while the user is around. It reads the stored hold, which holds
// from the moment Jaz starts, before the guard first runs.
func (s *Service) HeldBots() (map[string]bool, error) {
	since, err := s.Repo.AwaySince()
	if err != nil || since.IsZero() {
		return nil, err
	}
	items, err := s.Repo.ListLoops()
	if err != nil {
		return nil, err
	}
	held := map[string]bool{}
	for _, loop := range items {
		if loop.Status == StatusActive && loop.BotID != "" && !loop.KeepRunning {
			held[loop.BotID] = true
		}
	}
	return held, nil
}

// heldAway reports whether loop's runs wait for the user to come back.
func (s *Service) heldAway(loop Loop) bool {
	return s.away && loop.BotID != "" && !loop.KeepRunning
}

// tellAway has each bot with routines that hold while the user is away say
// they hold, or that they go on again.
func (s *Service) tellAway(away bool) {
	if s.Notice == nil {
		return
	}
	items, err := s.Repo.ListLoops()
	if err != nil {
		s.Log.Warn("away notice failed", "error", err)
		return
	}
	names := map[string][]string{}
	var bots []string
	for _, loop := range items {
		if loop.Status != StatusActive || loop.BotID == "" || loop.KeepRunning {
			continue
		}
		if names[loop.BotID] == nil {
			bots = append(bots, loop.BotID)
		}
		names[loop.BotID] = append(names[loop.BotID], loop.Name)
	}
	for _, bot := range bots {
		text := "Welcome back. My routines run again: " + strings.Join(names[bot], ", ") + "."
		if away {
			text = "You haven't written to Jaz for 3 days, so my routines wait until you're back: " + strings.Join(names[bot], ", ") + ". Your next message starts them again."
		}
		if err := s.Notice(bot, text); err != nil {
			s.Log.Warn("away notice failed", "bot", bot, "error", err)
		}
	}
}
