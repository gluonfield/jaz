package vault

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

type memVaultStore struct {
	mu      sync.Mutex
	records map[string]storage.VaultItemRecord
}

func newMemVaultStore() *memVaultStore {
	return &memVaultStore{records: map[string]storage.VaultItemRecord{}}
}

func (s *memVaultStore) ListVaultItems(_ context.Context, kind string) ([]storage.VaultItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]storage.VaultItem, 0, len(s.records))
	for _, rec := range s.records {
		if kind != "" && rec.Item.Kind != kind {
			continue
		}
		out = append(out, rec.Item)
	}
	return out, nil
}

func (s *memVaultStore) GetVaultItem(_ context.Context, id string) (storage.VaultItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[id]
	if !ok {
		return storage.VaultItem{}, storage.ErrVaultItemNotFound
	}
	return rec.Item, nil
}

func (s *memVaultStore) GetVaultItemRecord(_ context.Context, id string) (storage.VaultItemRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.records[id]
	if !ok {
		return storage.VaultItemRecord{}, storage.ErrVaultItemNotFound
	}
	return rec, nil
}

func (s *memVaultStore) CreateVaultItem(_ context.Context, record storage.VaultItemRecord) (storage.VaultItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[record.Item.ID] = record
	return record.Item, nil
}

func (s *memVaultStore) UpdateVaultItem(_ context.Context, record storage.VaultItemRecord) (storage.VaultItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.records[record.Item.ID]; !ok {
		return storage.VaultItem{}, storage.ErrVaultItemNotFound
	}
	s.records[record.Item.ID] = record
	return record.Item, nil
}

func (s *memVaultStore) DeleteVaultItem(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.records[id]; !ok {
		return storage.ErrVaultItemNotFound
	}
	delete(s.records, id)
	return nil
}

type fakeBrowserFiller struct {
	origin string
	calls  []BlindFillRequest
}

func (f *fakeBrowserFiller) ActivePageOrigin(_ context.Context, _ string) (string, error) {
	return f.origin, nil
}

func (f *fakeBrowserFiller) BlindFill(_ context.Context, _ string, req BlindFillRequest) (BlindFillResult, error) {
	f.calls = append(f.calls, req)
	slots := make([]string, 0, len(req.Fields))
	for _, field := range req.Fields {
		slots = append(slots, field.Slot)
	}
	return BlindFillResult{FilledSlots: slots}, nil
}

type fakeInteractionBroker struct {
	approveCard   bool
	approvals     []sessionevents.VaultApprovalSpec
	requests      []sessionevents.VaultRequestSpec
	requestItemID string
	requestCancel bool
}

func (b *fakeInteractionBroker) ConfirmVaultFill(_ context.Context, _ string, approval sessionevents.VaultApprovalSpec) (bool, error) {
	b.approvals = append(b.approvals, approval)
	return b.approveCard, nil
}

func (b *fakeInteractionBroker) RequestVaultItem(_ context.Context, _ string, req sessionevents.VaultRequestSpec) (string, bool, error) {
	b.requests = append(b.requests, req)
	return b.requestItemID, b.requestCancel, nil
}

func TestNormalizeOriginAndRFC6238TOTP(t *testing.T) {
	got, err := NormalizeOrigin("https://GitHub.com:443/login")
	if err != nil || got != "https://github.com" {
		t.Fatalf("unexpected normalized origin: %q err=%v", got, err)
	}
	got, err = NormalizeOrigin("http://localhost:3000/signin")
	if err != nil || got != "http://localhost:3000" {
		t.Fatalf("expected localhost HTTP allowed, got %q err=%v", got, err)
	}
	if _, err := NormalizeOrigin("http://example.com/login"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected plain HTTP non-loopback rejected, got %v", err)
	}
	if OriginMatchesLogin([]string{"https://github.com"}, "https://gist.github.com") {
		t.Fatal("subdomain must not match exact login origin")
	}
	if !OriginMatchesLogin([]string{"https://github.com"}, "https://github.com") {
		t.Fatal("exact login origin should match")
	}

	// RFC 6238 SHA-1 test vector: secret "12345678901234567890" (base32 GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ) at t=59 => 287082 (6 digits)
	code, err := GenerateTOTP("GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", time.Unix(59, 0).UTC())
	if err != nil {
		t.Fatalf("GenerateTOTP: %v", err)
	}
	if code != "287082" {
		t.Fatalf("expected RFC 6238 code 287082, got %q", code)
	}
}

func TestImportCSVDeduplicatesAndHandlesManagerFormats(t *testing.T) {
	svc := NewService(newMemVaultStore())
	csvPayload := strings.Join([]string{
		"name,url,username,password,totp",
		"GitHub,https://github.com/login,octocat,pass-1,JBSWY3DPEHPK3PXP",
		"GitHub Dup,https://github.com/settings,octocat,pass-2,",
		"Bad HTTP,http://insecure.example.com,user,pass-3,",
		"Local App,http://127.0.0.1:8080/login,dev,pass-4,",
	}, "\n")

	res, err := svc.ImportCSV(context.Background(), csvPayload)
	if err != nil {
		t.Fatalf("ImportCSV: %v", err)
	}
	if res.Imported != 2 || res.Skipped != 1 || res.Failed != 1 {
		t.Fatalf("unexpected import counts: %+v", res)
	}
	items, err := svc.List(context.Background(), storage.VaultKindLogin, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 imported logins, got %d", len(items))
	}
}

func TestMCPToolsBlindAutofillPoliciesAndZeroSecretLeakage(t *testing.T) {
	store := newMemVaultStore()
	fixedTime := time.Unix(59, 0).UTC()
	svc := NewService(store).WithClock(func() time.Time { return fixedTime })
	browser := &fakeBrowserFiller{origin: "https://github.com"}
	broker := &fakeInteractionBroker{}

	login, err := svc.Create(context.Background(), CreateInput{
		Kind:       storage.VaultKindLogin,
		Label:      "GitHub",
		Origins:    []string{"https://github.com"},
		Username:   "octocat",
		Password:   "gh-secret-pass",
		TOTPSecret: "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ",
	})
	if err != nil {
		t.Fatalf("create login: %v", err)
	}
	card, err := svc.Create(context.Background(), CreateInput{
		Kind:           storage.VaultKindCard,
		Label:          "Personal Visa",
		CardholderName: "Jane Doe",
		PAN:            "4242 4242 4242 4242",
		CardExpMonth:   12,
		CardExpYear:    2030,
		CVV:            "987",
	})
	if err != nil {
		t.Fatalf("create card: %v", err)
	}

	mcpServer := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	AddMCPTools(mcpServer, svc, browser, broker)

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return mcpServer }, &mcp.StreamableHTTPOptions{JSONResponse: true})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Set(mcpsession.HeaderName, "sess-vault-1")
		handler.ServeHTTP(w, r)
	}))
	defer ts.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: ts.URL}, nil)
	if err != nil {
		t.Fatalf("connect mcp client: %v", err)
	}
	defer session.Close()

	// 1. vault_list never leaks secrets
	listRes, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: ToolVaultList, Arguments: map[string]any{}})
	if err != nil || listRes.IsError {
		t.Fatalf("vault_list failed: err=%v res=%+v", err, listRes)
	}
	rawList, _ := json.Marshal(listRes)
	for _, secret := range []string{"gh-secret-pass", "GEZDGNBVGY3TQOJQ", "4242424242424242", "987"} {
		if strings.Contains(string(rawList), secret) {
			t.Fatalf("vault_list leaked secret %q", secret)
		}
	}

	// 2. vault_fill login succeeds silently on exact matching origin with TOTP, zero approval prompt
	fillRes, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: ToolVaultFill,
		Arguments: map[string]any{
			"item_id":         login.ID,
			"expected_origin": "https://github.com",
			"fields": map[string]string{
				"username": "ax:1",
				"password": "ax:2",
				"totp":     "ax:3",
			},
		},
	})
	if err != nil || fillRes.IsError {
		t.Fatalf("vault_fill login failed: err=%v res=%+v", err, fillRes)
	}
	rawFill, _ := json.Marshal(fillRes)
	if strings.Contains(string(rawFill), "gh-secret-pass") || strings.Contains(string(rawFill), "287082") {
		t.Fatalf("vault_fill response leaked secret: %s", string(rawFill))
	}
	if len(broker.approvals) != 0 {
		t.Fatalf("login on exact origin should autofill silently, got %d approvals", len(broker.approvals))
	}
	if len(browser.calls) != 1 || browser.calls[0].AllowPaymentIframes {
		t.Fatalf("expected 1 blind fill call with AllowPaymentIframes=false, got %+v", browser.calls)
	}

	// 3. vault_fill login rejects mismatched origin
	browser.origin = "https://evil.example.com"
	mismatchRes, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: ToolVaultFill,
		Arguments: map[string]any{
			"item_id":         login.ID,
			"expected_origin": "https://evil.example.com",
			"fields":          map[string]string{"password": "ax:2"},
		},
	})
	if err == nil && !mismatchRes.IsError {
		t.Fatalf("expected origin mismatch error, got %+v", mismatchRes)
	}
	if len(browser.calls) != 1 {
		t.Fatalf("origin mismatch must not call BlindFill")
	}

	// 4. vault_fill card requires approval; denied never calls BlindFill
	browser.origin = "https://checkout.shop.example"
	broker.approveCard = false
	deniedRes, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: ToolVaultFill,
		Arguments: map[string]any{
			"item_id":          card.ID,
			"expected_origin":  "https://checkout.shop.example",
			"checkout_summary": "£49.00 order",
			"fields": map[string]string{
				"cc_number": "ax:10",
				"cc_cvv":    "ax:11",
			},
		},
	})
	if err != nil || deniedRes.IsError {
		t.Fatalf("vault_fill card deny should return structured denied status: err=%v res=%+v", err, deniedRes)
	}
	if len(broker.approvals) != 1 || broker.approvals[0].MerchantOrigin != "https://checkout.shop.example" || broker.approvals[0].CheckoutSummary != "£49.00 order" {
		t.Fatalf("unexpected approval spec: %+v", broker.approvals)
	}
	if len(browser.calls) != 1 {
		t.Fatalf("denied card fill must never call BlindFill")
	}

	// 5. vault_fill card approved calls BlindFill with AllowPaymentIframes=true and zero secret echo
	broker.approveCard = true
	approvedRes, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: ToolVaultFill,
		Arguments: map[string]any{
			"item_id":         card.ID,
			"expected_origin": "https://checkout.shop.example",
			"fields": map[string]string{
				"cc_number": "ax:10",
				"cc_cvv":    "ax:11",
			},
		},
	})
	if err != nil || approvedRes.IsError {
		t.Fatalf("vault_fill card approved failed: err=%v res=%+v", err, approvedRes)
	}
	rawCardFill, _ := json.Marshal(approvedRes)
	if strings.Contains(string(rawCardFill), "4242424242424242") || strings.Contains(string(rawCardFill), "987") {
		t.Fatalf("approved card fill leaked secrets: %s", string(rawCardFill))
	}
	if len(browser.calls) != 2 || !browser.calls[1].AllowPaymentIframes {
		t.Fatalf("expected approved card fill with AllowPaymentIframes=true, got %+v", browser.calls)
	}

	// 6. vault_request_item returns opaque handle on save and cancelled on cancel
	broker.requestItemID = login.ID
	broker.requestCancel = false
	reqRes, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: ToolVaultRequestItem,
		Arguments: map[string]any{
			"kind":   "login",
			"domain": "github.com",
			"label":  "GitHub",
		},
	})
	if err != nil || reqRes.IsError {
		t.Fatalf("vault_request_item failed: err=%v res=%+v", err, reqRes)
	}
	rawReq, _ := json.Marshal(reqRes)
	if !strings.Contains(string(rawReq), login.ID) || strings.Contains(string(rawReq), "gh-secret-pass") {
		t.Fatalf("unexpected vault_request_item output: %s", string(rawReq))
	}
}
