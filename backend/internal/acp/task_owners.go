package acp

import (
	"slices"

	"github.com/wins/jaz/backend/internal/storage"
)

// taskOwners records whom the agent's background tasks report to: what the
// turn in which each task started owes, until the work that takes in the
// task's end names it. Claude Code tells the model a task finished in the work
// that runs when it finishes or in the work right after; it begins a run of
// its own to tell it of a task that finished while no work ran.
type taskOwners struct {
	live  map[string]storage.Turn
	ended []endedTask
}

type endedTask struct {
	id   string
	owed storage.Turn
	// idle records that the task finished while no work ran; left counts the
	// works that may still end without naming it.
	idle bool
	left int
}

// has reports whether task id runs with a known owner.
func (o *taskOwners) has(id string) bool {
	_, known := o.live[id]
	return known
}

// start records what the turn in which task id starts owes.
func (o *taskOwners) start(id string, owed storage.Turn) {
	if o.live == nil {
		o.live = make(map[string]storage.Turn)
	}
	o.live[id] = owed
}

// end records that task id ended. It waits for the work that takes its end
// in, which, while working, may be the work that runs.
func (o *taskOwners) end(id string, working bool) {
	owed, known := o.live[id]
	delete(o.live, id)
	if !known {
		return
	}
	task := endedTask{id: id, owed: owed, idle: true, left: 1}
	if working {
		task.idle, task.left = false, 2
	}
	o.ended = append(o.ended, task)
}

// take returns what each ended task in ids owes, and forgets those tasks.
func (o *taskOwners) take(ids []string) []storage.Turn {
	var owed []storage.Turn
	o.ended = slices.DeleteFunc(o.ended, func(task endedTask) bool {
		taken := slices.Contains(ids, task.id)
		if taken {
			owed = append(owed, task.owed)
		}
		return taken
	})
	return owed
}

// workEnded forgets the ended tasks that no later work takes in.
func (o *taskOwners) workEnded() {
	kept := o.ended[:0]
	for _, task := range o.ended {
		task.left--
		if task.left > 0 {
			kept = append(kept, task)
		}
	}
	o.ended = kept
}

// idle is what the tasks that finished while no work ran owe: a run the agent
// begins on its own tells the model of them.
func (o *taskOwners) idle() storage.Turn {
	owed := storage.Turn{AllowSilence: true}
	for _, task := range o.ended {
		if task.idle {
			owed = owed.Owing(task.owed)
		}
	}
	return owed
}
