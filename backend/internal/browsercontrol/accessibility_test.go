package browsercontrol

import (
	"strings"
	"testing"
)

func TestAXRevisionPreservesIdentityAndHierarchy(t *testing.T) {
	frame := axFrame{cdpFrame: cdpFrame{ID: "main", LoaderID: "document"}}
	nodes := []axNode{
		{ID: "root", Role: axValue{Value: "RootWebArea"}, ChildIDs: []string{"ignored"}},
		{ID: "ignored", ParentID: "root", Ignored: true, ChildIDs: []string{"button"}},
		{ID: "button", ParentID: "ignored", Role: axValue{Value: "button"}, Name: axValue{Value: "Save"}, BackendID: 17},
	}
	first := axState{indices: map[string]int{}, targets: map[string]axTarget{}}
	first.appendTree(&axTree{Frame: frame, Nodes: nodes}, nil, 0, -1)
	if len(first.lines) != 2 || first.lines[1].Parent != first.lines[0].Index || first.lines[1].Text != "\t1 button \"Save\"" {
		t.Fatalf("ignored ancestor was not flattened correctly: %+v", first.lines)
	}
	second := axState{next: first.next, indices: map[string]int{}, targets: map[string]axTarget{}}
	nodes[2].Name.Value = "Saved"
	second.appendTree(&axTree{Frame: frame, Nodes: nodes}, first.indices, 0, -1)
	if got := renderAXRevision(first.lines, second.lines, false); got != "Changed: \t1 button \"Saved\"" {
		t.Fatalf("changed name did not preserve index: %s", got)
	}
	if second.targets["ax:1"].BackendID != 17 {
		t.Fatal("accessibility index lost its DOM identity")
	}
	frame.LoaderID = "replacement-document"
	third := axState{next: second.next, indices: map[string]int{}, targets: map[string]axTarget{}}
	third.appendTree(&axTree{Frame: frame, Nodes: nodes}, second.indices, 0, -1)
	if _, exists := third.targets["ax:1"]; exists {
		t.Fatal("an old index was reused for another document")
	}
}

func TestAXRevisionReportsRemovalAndUsesFullTreeForMoves(t *testing.T) {
	before := []axLine{{Index: 0, Parent: -1, Text: "0 root"}, {Index: 1, Parent: 0, Text: "\t1 button A"}, {Index: 2, Parent: 0, Text: "\t2 button B"}}
	if got := renderAXRevision(before, before, false); got != "Accessibility tree unchanged." {
		t.Fatal(got)
	}
	if got := renderAXRevision(before, before[:2], false); got != "Removed: \t2 button B" {
		t.Fatal(got)
	}
	reordered := []axLine{before[0], before[2], before[1]}
	if got := renderAXRevision(before, reordered, false); got != "0 root\n\t2 button B\n\t1 button A" {
		t.Fatalf("reordering was not represented: %s", got)
	}
	reparented := append([]axLine(nil), before...)
	reparented[2].Parent = 1
	if got := renderAXRevision(before, reparented, false); !strings.HasPrefix(got, "0 root\n") {
		t.Fatalf("reparenting requires full hierarchy: %s", got)
	}
}

func TestAXChildFrameIsNestedUnderItsOwner(t *testing.T) {
	state := axState{indices: map[string]int{}, targets: map[string]axTarget{}}
	parent := axFrame{cdpFrame: cdpFrame{ID: "parent", LoaderID: "p"}}
	child := axFrame{cdpFrame: cdpFrame{ID: "child", LoaderID: "c"}, ParentID: "parent", OwnerID: 20}
	state.appendTree(&axTree{
		Frame: parent,
		Nodes: []axNode{
			{ID: "root", Role: axValue{Value: "RootWebArea"}, ChildIDs: []string{"frame", "button", "hidden"}},
			{ID: "frame", ParentID: "root", Role: axValue{Value: "Iframe"}, BackendID: 20},
			{ID: "button", ParentID: "root", Role: axValue{Value: "button"}},
			{ID: "hidden", ParentID: "root", Ignored: true, BackendID: 30},
		},
		Children: map[int64]*axTree{
			20: {Frame: child, Nodes: []axNode{{ID: "root", Role: axValue{Value: "RootWebArea"}}}},
			30: {Nodes: []axNode{{ID: "hidden", Role: axValue{Value: "button"}, Name: axValue{Value: "Hidden frame action"}}}},
		},
	}, nil, 0, -1)
	if len(state.lines) != 4 || state.lines[2].Parent != 1 || state.lines[2].Text != "\t\t2 RootWebArea" || state.lines[3].Text != "\t3 button" {
		t.Fatalf("child document was not nested under its exposed iframe: %+v", state.lines)
	}
}
