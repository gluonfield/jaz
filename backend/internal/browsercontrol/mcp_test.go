package browsercontrol

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
)

type recordingBackend struct {
	inputs  []ActionInput
	outputs map[string]ActionOutput
}

func (b *recordingBackend) Call(_ context.Context, input ActionInput) (ActionOutput, error) {
	b.inputs = append(b.inputs, input)
	return b.outputs[input.Action], nil
}

func TestClaimTabUsesExplicitTabAndSession(t *testing.T) {
	backend := &recordingBackend{outputs: map[string]ActionOutput{
		ActionClaimTab: {Status: "ok", Text: "claimed"},
	}}
	_, out, err := (directTools{backend: backend}).ClaimTab(context.Background(), &mcp.CallToolRequest{
		Extra: &mcp.RequestExtra{Header: mcpsession.Header("browser-session-1")},
	}, ClaimTabInput{TabID: "42"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "claimed" || len(backend.inputs) != 1 {
		t.Fatalf("out=%#v inputs=%#v", out, backend.inputs)
	}
	if input := backend.inputs[0]; input.Action != ActionClaimTab || input.TabID != "42" || input.Session != "browser-session-1" {
		t.Fatalf("claim input=%#v", input)
	}
}

func TestUnavailableBackendReportsMissingBridge(t *testing.T) {
	session := connectBrowserMCP(t, nil)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      ToolStatus,
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("result = %#v, want error result", result)
	}
	if len(result.Content) == 0 || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "browser backend is not connected") {
		t.Fatalf("content = %#v", result.Content)
	}
}

func TestFormInputSchemaAcceptsEverySupportedValueType(t *testing.T) {
	backend := &recordingBackend{outputs: map[string]ActionOutput{
		ActionFormInput: {Status: "ok", Text: "set"},
	}}
	session := connectBrowserMCP(t, backend)
	for _, value := range []any{"prepared narrative", 17, true} {
		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
			Name:      ToolFormInput,
			Arguments: map[string]any{"ref": "p1:e2", "value": value},
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError {
			t.Fatalf("value %#v rejected: %#v", value, result.Content)
		}
	}
	formCalls := make([]ActionInput, 0, 3)
	for _, input := range backend.inputs {
		if input.Action == ActionFormInput {
			formCalls = append(formCalls, input)
		}
	}
	if len(formCalls) != 3 {
		t.Fatalf("form calls = %#v", formCalls)
	}
}

func TestToolPassesSessionAndImageContent(t *testing.T) {
	backend := &recordingBackend{outputs: map[string]ActionOutput{
		ActionScreenshot: {
			Status:        "ok",
			Text:          "captured",
			ImageData:     []byte("image"),
			ImageMIMEType: "image/png",
		},
	}}
	result, _, err := (directTools{backend: backend}).Screenshot(context.Background(), &mcp.CallToolRequest{
		Extra: &mcp.RequestExtra{Header: mcpsession.Header("browser-session-1")},
	}, EmptyInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(backend.inputs) != 1 || backend.inputs[0].Session != "browser-session-1" || backend.inputs[0].Action != ActionScreenshot {
		t.Fatalf("inputs = %#v", backend.inputs)
	}
	if len(result.Content) != 2 {
		t.Fatalf("content = %#v", result.Content)
	}
	if text, ok := result.Content[0].(*mcp.TextContent); !ok || text.Text != "captured" {
		t.Fatalf("text content = %#v", result.Content[0])
	}
	if image, ok := result.Content[1].(*mcp.ImageContent); !ok || image.MIMEType != "image/png" || string(image.Data) != "image" {
		t.Fatalf("image content = %#v", result.Content[1])
	}
}

func TestFormInputRequiresAndPassesOpaqueRef(t *testing.T) {
	backend := &recordingBackend{outputs: map[string]ActionOutput{
		ActionFormInput: {Status: "ok", Text: "set"},
	}}
	_, out, err := (directTools{backend: backend}).FormInput(context.Background(), &mcp.CallToolRequest{}, FormInput{
		Ref:   "f0:p1:e4",
		Value: 17,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "set" {
		t.Fatalf("out = %#v", out)
	}
	input := backend.inputs[0]
	if input.Action != ActionFormInput || input.Ref != "f0:p1:e4" || input.Value != 17 {
		t.Fatalf("input = %#v", input)
	}
}

func TestContentResultTruncatesHugeText(t *testing.T) {
	result := contentResult(ActionOutput{Text: strings.Repeat("x", browserToolTextLimit+100)})
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content = %#v", result.Content)
	}
	if len(text.Text) > browserToolTextLimit+80 || !strings.Contains(text.Text, "[truncated: browser output exceeded") {
		t.Fatalf("text length=%d suffix=%q", len(text.Text), text.Text[len(text.Text)-60:])
	}
}

func TestScrollAndWaitRejectInvalidBounds(t *testing.T) {
	tools := directTools{backend: &recordingBackend{}}
	if _, _, err := tools.Scroll(context.Background(), &mcp.CallToolRequest{}, ScrollInput{Direction: "dwon"}); err == nil {
		t.Fatal("invalid scroll direction was accepted")
	}
	if _, _, err := tools.Scroll(context.Background(), &mcp.CallToolRequest{}, ScrollInput{Amount: -1}); err == nil {
		t.Fatal("negative scroll amount was accepted")
	}
	if _, _, err := tools.Scroll(context.Background(), &mcp.CallToolRequest{}, ScrollInput{Amount: browserScrollLimit + 1}); err == nil {
		t.Fatal("oversized scroll amount was accepted")
	}
	if _, _, err := tools.Wait(context.Background(), &mcp.CallToolRequest{}, WaitInput{TimeoutMS: -1}); err == nil {
		t.Fatal("negative wait timeout was accepted")
	}
	if _, _, err := tools.Wait(context.Background(), &mcp.CallToolRequest{}, WaitInput{TimeoutMS: browserWaitLimit + 1}); err == nil {
		t.Fatal("oversized wait timeout was accepted")
	}
	if _, _, err := tools.Wait(context.Background(), &mcp.CallToolRequest{}, WaitInput{Text: strings.Repeat("x", browserQueryLimit+1)}); err == nil {
		t.Fatal("oversized wait text was accepted")
	}
}

func TestDirectToolsRejectOversizedInputs(t *testing.T) {
	tools := directTools{backend: &recordingBackend{}}
	request := &mcp.CallToolRequest{}
	if _, _, err := tools.ClaimTab(context.Background(), request, ClaimTabInput{TabID: strings.Repeat("x", browserTabIDLimit+1)}); err == nil {
		t.Fatal("oversized tab id was accepted")
	}
	if _, _, err := tools.Navigate(context.Background(), request, NavigateInput{URL: strings.Repeat("x", browserURLLimit+1)}); err == nil {
		t.Fatal("oversized URL was accepted")
	}
	if _, _, err := tools.Find(context.Background(), request, FindInput{Query: strings.Repeat("x", browserQueryLimit+1)}); err == nil {
		t.Fatal("oversized find query was accepted")
	}
	if _, _, err := tools.Click(context.Background(), request, RefInput{Ref: strings.Repeat("x", stateRefLimit+1)}); err == nil {
		t.Fatal("oversized ref was accepted")
	}
	if _, _, err := tools.FormInput(context.Background(), request, FormInput{Ref: "p1:e1", Value: strings.Repeat("x", browserFormValueLimit+1)}); err == nil {
		t.Fatal("oversized form value was accepted")
	}
	if _, _, err := tools.Key(context.Background(), request, KeyInput{Key: strings.Repeat("x", browserKeyLimit+1)}); err == nil {
		t.Fatal("oversized key was accepted")
	}
}

func TestDecodePageStateBoundsStructuredContent(t *testing.T) {
	elements := make([]StateElement, stateDataElementLimit+10)
	for i := range elements {
		elements[i] = StateElement{
			Ref:        fmt.Sprintf("p1:e%d", i+1),
			Name:       strings.Repeat("n", stateNameLimit+10),
			Text:       strings.Repeat("e", stateElementTextLimit+10),
			Href:       "https://example.com/" + strings.Repeat("h", stateHrefLimit),
			Value:      " \n" + strings.Repeat("v", stateValueLimit+10),
			Validation: strings.Repeat("x", stateValidationLimit+10),
		}
	}
	data, err := json.Marshal(PageState{
		URL:      strings.Repeat("u", 1100),
		Title:    strings.Repeat("t", 600),
		Text:     "a" + strings.Repeat("é", stateDataTextLimit),
		Elements: elements,
	})
	if err != nil {
		t.Fatal(err)
	}
	state, ok := decodePageState(data)
	if !ok {
		t.Fatal("page state did not decode")
	}
	if len(state.URL) != 1000 || len(state.Title) != 500 || len(state.Text) > stateDataTextLimit || !utf8.ValidString(state.Text) {
		t.Fatalf("state bounds = url:%d title:%d text:%d valid:%v", len(state.URL), len(state.Title), len(state.Text), utf8.ValidString(state.Text))
	}
	if len(state.Elements) != stateDataElementLimit {
		t.Fatalf("elements = %d", len(state.Elements))
	}
	element := state.Elements[0]
	if len(element.Name) != stateNameLimit || len(element.Text) != stateElementTextLimit ||
		len(element.Href) != stateHrefLimit || len(element.Value) != stateValueLimit ||
		len(element.Validation) != stateValidationLimit {
		t.Fatalf("element bounds = %#v", element)
	}
	if !strings.HasPrefix(element.Value, " \n") {
		t.Fatalf("structured value whitespace was changed: %q", element.Value[:10])
	}
}

func TestDecodePageStateRejectsOversizedJSONAndRefs(t *testing.T) {
	if _, ok := decodePageState(json.RawMessage(strings.Repeat(" ", stateJSONLimit+1))); ok {
		t.Fatal("oversized page JSON was accepted")
	}
	data, err := json.Marshal(PageState{Elements: []StateElement{
		{Ref: strings.Repeat("r", stateRefLimit+1)},
		{Ref: "p1:e2"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	state, ok := decodePageState(data)
	if !ok || len(state.Elements) != 1 || state.Elements[0].Ref != "p1:e2" {
		t.Fatalf("state = %#v ok=%v", state, ok)
	}
}

func TestDecodePageStateStripsPasswordTextAndValue(t *testing.T) {
	data, err := json.Marshal(PageState{Elements: []StateElement{{
		Ref:       "p1:e1",
		Name:      "Password",
		Text:      "secret",
		InputType: "password",
		Value:     "secret",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	state, ok := decodePageState(data)
	if !ok || len(state.Elements) != 1 {
		t.Fatalf("state = %#v ok=%v", state, ok)
	}
	element := state.Elements[0]
	if element.Name != "Password" || element.Text != "" || element.Value != "" {
		t.Fatalf("password element was not sanitized: %#v", element)
	}
}

func TestBoundBrowserTabsBoundsStructuredContent(t *testing.T) {
	tabs := make([]BrowserTab, browserTabLimit+2)
	tabs[0] = BrowserTab{ID: strings.Repeat("x", browserTabIDLimit+1)}
	for i := 1; i < len(tabs); i++ {
		tabs[i] = BrowserTab{
			ID:        fmt.Sprintf("tab-%d", i),
			Title:     strings.Repeat("t", 600),
			URL:       "https://example.com/" + strings.Repeat("u", 2100),
			Ownership: strings.Repeat("o", 110),
		}
	}
	got := boundBrowserTabs(tabs)
	if len(got) != browserTabLimit {
		t.Fatalf("tabs = %d", len(got))
	}
	if got[0].ID != "tab-1" || len(got[0].Title) != 500 || len(got[0].URL) != 2000 || len(got[0].Ownership) != 100 {
		t.Fatalf("first tab = %#v", got[0])
	}
}

func TestActionDoesNotPerformImplicitPageRead(t *testing.T) {
	backend := &recordingBackend{outputs: map[string]ActionOutput{
		ActionClick: {Status: "ok", Text: "clicked"},
	}}
	result, out, err := (directTools{backend: backend}).Click(
		context.Background(),
		&mcp.CallToolRequest{},
		RefInput{Ref: "p1:e2"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "clicked" || len(backend.inputs) != 1 || backend.inputs[0].Action != ActionClick {
		t.Fatalf("out = %#v", out)
	}
	if text := result.Content[0].(*mcp.TextContent).Text; text != "clicked" {
		t.Fatalf("content = %q", text)
	}
}

func connectBrowserMCP(t *testing.T, backend Backend) *mcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "browsercontrol", Version: "test"}, nil)
	AddMCPTools(server, backend)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}
