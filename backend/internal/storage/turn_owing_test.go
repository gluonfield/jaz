package storage_test

import (
	"reflect"
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
)

func TestWorkATurnOwingTwoBotsLeavesRunningOwesBoth(t *testing.T) {
	x, y := &storage.TurnOutput{ReplyTo: "x"}, &storage.TurnOutput{ReplyTo: "y"}
	run := storage.Turn{AllowSilence: true}
	for _, owed := range []storage.Turn{{NotifyParent: true, ParentOutput: x}, {NotifyParent: true, ParentOutput: y}, {NotifyParent: true, ParentOutput: y}} {
		run = run.Owing(owed)
	}
	owes := []storage.Recipient{{Parent: true, ReplyTo: "y"}}
	if want := (storage.Turn{AllowSilence: true, NotifyParent: true, ParentOutput: x, Owes: owes}); !reflect.DeepEqual(run, want) {
		t.Fatalf("run = %+v, want it to answer x and, once, y", run)
	}
	if later, want := (storage.Turn{}).Owing(run.Owed()), (storage.Turn{NotifyParent: true, ParentOutput: x, Owes: owes}); !reflect.DeepEqual(later, want) {
		t.Fatalf("the turn that reports work the run left running owes %+v, want %+v", later, want)
	}
}
