package browserworker

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

type browserContract struct {
	Protocol  string   `json:"protocol"`
	Actions   []string `json:"actions"`
	PageState struct {
		Fields        []string `json:"fields"`
		ElementFields []string `json:"element_fields"`
	} `json:"page_state"`
	PageExtraction struct {
		Fields         []string `json:"fields"`
		CoverageFields []string `json:"coverage_fields"`
		ItemFields     []string `json:"item_fields"`
	} `json:"page_extraction"`
}

func TestBrowserExtensionContract(t *testing.T) {
	contract := loadBrowserContract(t)
	if contract.Protocol != ExtensionProtocol {
		t.Fatalf("protocol = %q, want %q", contract.Protocol, ExtensionProtocol)
	}
	if !reflect.DeepEqual(contract.Actions, SupportedExtensionActions()) {
		t.Fatalf("actions = %#v, want %#v", contract.Actions, SupportedExtensionActions())
	}
	if !reflect.DeepEqual(contract.PageState.Fields, jsonFields(PageState{})) {
		t.Fatalf("page fields = %#v, want %#v", contract.PageState.Fields, jsonFields(PageState{}))
	}
	if !reflect.DeepEqual(contract.PageState.ElementFields, jsonFields(StateElement{})) {
		t.Fatalf("element fields = %#v, want %#v", contract.PageState.ElementFields, jsonFields(StateElement{}))
	}
	if !reflect.DeepEqual(contract.PageExtraction.Fields, jsonFields(PageExtraction{})) {
		t.Fatalf("extraction fields = %#v, want %#v", contract.PageExtraction.Fields, jsonFields(PageExtraction{}))
	}
	if !reflect.DeepEqual(contract.PageExtraction.CoverageFields, jsonFields(ExtractCoverage{})) {
		t.Fatalf("coverage fields = %#v, want %#v", contract.PageExtraction.CoverageFields, jsonFields(ExtractCoverage{}))
	}
	if !reflect.DeepEqual(contract.PageExtraction.ItemFields, jsonFields(ExtractedItem{})) {
		t.Fatalf("item fields = %#v, want %#v", contract.PageExtraction.ItemFields, jsonFields(ExtractedItem{}))
	}
}

func loadBrowserContract(t *testing.T) browserContract {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("missing caller path")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "..", "docs", "browser-extension-contract.json"))
	if err != nil {
		t.Fatal(err)
	}
	var contract browserContract
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatal(err)
	}
	return contract
}

func jsonFields(value any) []string {
	t := reflect.TypeOf(value)
	out := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		tag := strings.TrimSpace(t.Field(i).Tag.Get("json"))
		name, _, _ := strings.Cut(tag, ",")
		if name != "" && name != "-" {
			out = append(out, name)
		}
	}
	return out
}
