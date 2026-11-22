package toolsearch

import (
	"reflect"
	"testing"
)

func TestToolSearchTokenizesLikeCodexEnglishBM25(t *testing.T) {
	if got := tokenizeToolSearch("i me my myself we our ours ourselves you you're you've you'll you'd"); len(got) != 0 {
		t.Fatalf("stopword tokens = %v, want none", got)
	}
	if got := tokenizeToolSearch("space,station 42 1337 3.14"); !reflect.DeepEqual(got, []string{"space", "station", "42", "1337", "3.14"}) {
		t.Fatalf("punctuation/number tokens = %v", got)
	}
	if got := tokenizeToolSearch("connection connections connective connected connecting connect"); !reflect.DeepEqual(got, []string{"connect", "connect", "connect", "connect", "connect", "connect"}) {
		t.Fatalf("stemmed tokens = %v", got)
	}
}
