package mcpdeferred

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
)

const QueryParam = "jaz_deferred_tools"

var gatewayTools = json.RawMessage(`{"tools":[{"name":"search_tools","description":"Find available tools by English keywords (for example: command, memory, thread, browser, email). Returns the original tool names, descriptions and complete input schemas. Use an empty query to browse; follow next_offset for more results. Discover a tool before calling it with call_tool. All tools remain available through this catalogue.","inputSchema":{"type":"object","properties":{"query":{"type":"string"},"offset":{"type":"integer","minimum":0},"limit":{"type":"integer","minimum":1,"maximum":10}},"additionalProperties":false},"annotations":{"readOnlyHint":true}},{"name":"call_tool","description":"Execute an available tool discovered with search_tools. Pass its exact name and arguments matching its input schema. The original permissions, session context, validation, results and errors apply. This does not bypass user approval or access restrictions.","inputSchema":{"type":"object","properties":{"name":{"type":"string"},"arguments":{"type":"object","additionalProperties":true}},"required":["name","arguments"],"additionalProperties":false}}]}`)

type message struct {
	JSONRPC string                     `json:"jsonrpc"`
	ID      json.RawMessage            `json:"id,omitempty"`
	Method  string                     `json:"method,omitempty"`
	Params  map[string]json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage            `json:"result,omitempty"`
	Error   json.RawMessage            `json:"error,omitempty"`
}

type handler struct{ next http.Handler }

// Wrap is opt-in per MCP connection. It never changes the underlying tool
// surface: search lists it, and dispatch goes through its normal SDK handler.
func Wrap(next http.Handler) http.Handler { return &handler{next: next} }

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	next := h.next
	if r.Method != http.MethodPost || r.URL.Query().Get(QueryParam) != "1" {
		next.ServeHTTP(w, r)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read MCP request", http.StatusBadRequest)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	var request message
	if json.Unmarshal(body, &request) != nil || len(request.ID) == 0 {
		next.ServeHTTP(w, r)
		return
	}
	switch request.Method {
	case "tools/list":
		response := list(next, r, request, "")
		if response.ok() {
			response.replaceResult(request.ID, gatewayTools)
		}
		response.write(w)
	case "tools/call":
		var name string
		_ = json.Unmarshal(request.Params["name"], &name)
		switch name {
		case "search_tools":
			search(next, w, r, request)
		case "call_tool":
			var input struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			var arguments map[string]json.RawMessage
			if !decodeArguments(request.Params["arguments"], &input, "name", "arguments") || strings.TrimSpace(input.Name) == "" || json.Unmarshal(input.Arguments, &arguments) != nil || arguments == nil {
				invalid(w, request.ID, "call_tool requires a name and an arguments object")
				return
			}
			request.Params["name"], _ = json.Marshal(strings.TrimSpace(input.Name))
			request.Params["arguments"] = input.Arguments
			next.ServeHTTP(w, rewritten(r, request))
		default:
			next.ServeHTTP(w, r)
		}
	default:
		next.ServeHTTP(w, r)
	}
}

func rewritten(r *http.Request, request message) *http.Request {
	body, _ := json.Marshal(request)
	copy := r.Clone(r.Context())
	copy.Body = io.NopCloser(bytes.NewReader(body))
	copy.ContentLength = int64(len(body))
	return copy
}

type response struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (r *response) Header() http.Header         { return r.header }
func (r *response) WriteHeader(status int)      { r.status = status }
func (r *response) Write(b []byte) (int, error) { return r.body.Write(b) }

func list(next http.Handler, r *http.Request, request message, cursor string) *response {
	request.Method = "tools/list"
	request.Params = map[string]json.RawMessage{}
	if cursor != "" {
		request.Params["cursor"], _ = json.Marshal(cursor)
	}
	result := &response{header: make(http.Header), status: http.StatusOK}
	next.ServeHTTP(result, rewritten(r, request))
	return result
}

func (r *response) ok() bool {
	var envelope message
	return r.status == http.StatusOK && json.Unmarshal(r.body.Bytes(), &envelope) == nil && len(envelope.Error) == 0 && len(envelope.Result) != 0
}

func (r *response) replaceResult(id, result json.RawMessage) {
	r.body.Reset()
	_ = json.NewEncoder(&r.body).Encode(message{JSONRPC: "2.0", ID: id, Result: result})
	r.header.Set("Content-Type", "application/json")
	r.header.Del("Content-Length")
}

func (r *response) write(w http.ResponseWriter) {
	for k, values := range r.header {
		w.Header()[k] = values
	}
	w.WriteHeader(r.status)
	_, _ = w.Write(r.body.Bytes())
}

func invalid(w http.ResponseWriter, id json.RawMessage, reason string) {
	w.Header().Set("Content-Type", "application/json")
	err, _ := json.Marshal(map[string]any{"code": -32602, "message": reason})
	_ = json.NewEncoder(w).Encode(message{JSONRPC: "2.0", ID: id, Error: err})
}

func decodeArguments(raw json.RawMessage, input any, allowed ...string) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return false
	}
	for name, value := range fields {
		if !slices.Contains(allowed, name) || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return false
		}
	}
	return json.Unmarshal(raw, input) == nil
}

func search(next http.Handler, w http.ResponseWriter, r *http.Request, request message) {
	input := struct {
		Query  string `json:"query"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}{Limit: 5}
	if !decodeArguments(request.Params["arguments"], &input, "query", "offset", "limit") || input.Offset < 0 || input.Limit < 1 || input.Limit > 10 {
		invalid(w, request.ID, "search_tools requires offset >= 0 and limit between 1 and 10")
		return
	}
	matches := make([]json.RawMessage, 0)
	var last *response
	cursor := ""
	seen := map[string]bool{}
	for {
		last = list(next, r, request, cursor)
		if !last.ok() {
			last.write(w)
			return
		}
		var envelope message
		_ = json.Unmarshal(last.body.Bytes(), &envelope)
		var page struct {
			Tools      []json.RawMessage `json:"tools"`
			NextCursor string            `json:"nextCursor"`
		}
		if json.Unmarshal(envelope.Result, &page) != nil {
			invalid(w, request.ID, "invalid tool catalogue")
			return
		}
		for _, tool := range page.Tools {
			var spec struct{ Name, Description string }
			_ = json.Unmarshal(tool, &spec)
			text := strings.ToLower(spec.Name + " " + spec.Description)
			if slices.ContainsFunc(strings.Fields(strings.ToLower(input.Query)), func(word string) bool { return strings.Contains(text, word) }) || strings.TrimSpace(input.Query) == "" {
				matches = append(matches, tool)
			}
		}
		cursor = page.NextCursor
		if cursor == "" {
			break
		}
		if seen[cursor] || r.Context().Err() != nil {
			invalid(w, request.ID, "tool catalogue pagination interrupted")
			return
		}
		seen[cursor] = true
	}
	start := min(input.Offset, len(matches))
	end := min(start+input.Limit, len(matches))
	result := map[string]any{"tools": matches[start:end], "total_matches": len(matches)}
	if end < len(matches) {
		result["next_offset"] = end
	}
	text, _ := json.Marshal(result)
	payload, _ := json.Marshal(map[string]any{"content": []map[string]string{{"type": "text", "text": string(text)}}, "structuredContent": result})
	last.replaceResult(request.ID, payload)
	last.write(w)
}
