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
// with the user's next message. Each bot tells the user once when its
// routines hold and once when they go on; a routine's own status never
// changes.
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
	s.away = !last.IsZero() && s.now().Sub(last) >= awayAfter
	if held {
		s.away = !last.After(since)
	}
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

// heldAway reports whether loop's runs wait for the user to come back.
func (s *Service) heldAway(loop Loop) bool {
	return s.away && loop.BotID != "" && !loop.KeepRunning
}

// tellAway has each bot with routines that hold while the user is away say
// they hold, or that they go on again.
func (s *Service) tellAway(away bool) {
	items, err := s.Repo.ListLoops()
	if err != nil || s.Notice == nil {
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
