package vault

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	ToolVaultList        = "vault_list"
	ToolVaultFill        = "vault_fill"
	ToolVaultRequestItem = "vault_request_item"
)

var mcpToolNames = []string{
	ToolVaultList,
	ToolVaultFill,
	ToolVaultRequestItem,
}

func MCPToolNames() []string {
	return append([]string(nil), mcpToolNames...)
}

type BlindFillField struct {
	Slot  string
	Ref   string
	Value string
}

type BlindFillRequest struct {
	ExpectedOrigin      string
	AllowPaymentIframes bool
	Fields              []BlindFillField
}

type BlindFillResult struct {
	FilledSlots []string
}

type BrowserFiller interface {
	ActivePageOrigin(ctx context.Context, sessionID string) (string, error)
	BlindFill(ctx context.Context, sessionID string, req BlindFillRequest) (BlindFillResult, error)
}

type InteractionBroker interface {
	ConfirmVaultFill(ctx context.Context, sessionID string, approval sessionevents.VaultApprovalSpec) (bool, error)
	RequestVaultItem(ctx context.Context, sessionID string, req sessionevents.VaultRequestSpec) (itemID string, cancelled bool, err error)
}

type ListInput struct {
	Domain string `json:"domain,omitempty" jsonschema:"Optional website domain or origin to filter login items."`
	Kind   string `json:"kind,omitempty" jsonschema:"Optional vault item kind: login, card, or identity."`
}

type ListOutput struct {
	Items []storage.VaultItem `json:"items"`
}

type FillInput struct {
	ItemID          string            `json:"item_id" jsonschema:"Opaque vault item id returned by vault_list or vault_request_item."`
	Fields          map[string]string `json:"fields" jsonschema:"Map from semantic slot (username, password, totp, cc_number, cc_exp_month, cc_exp_year, cc_exp_mmyy, cc_cvv, cc_name, billing_address, email, full_name, phone, address_lines, postal_code, country) to browser ref (ax:INDEX or ref=...)."`
	ExpectedOrigin  string            `json:"expected_origin" jsonschema:"Expected top-level origin of the active browser tab (e.g. https://example.com)."`
	CheckoutSummary string            `json:"checkout_summary,omitempty" jsonschema:"Short summary of the purchase/total shown to the user when approving a card fill."`
}

type FillOutput struct {
	Status      string   `json:"status"`
	FilledSlots []string `json:"filled_slots,omitempty"`
	Reason      string   `json:"reason,omitempty"`
}

type RequestItemInput struct {
	Kind   string `json:"kind" jsonschema:"Kind of vault item needed: login, card, or identity."`
	Domain string `json:"domain,omitempty" jsonschema:"Website origin or domain for a login item."`
	Label  string `json:"label,omitempty" jsonschema:"Suggested label for the new vault item."`
	Reason string `json:"reason,omitempty" jsonschema:"Short explanation shown to the user in the secure form card."`
}

type RequestItemOutput struct {
	Status string             `json:"status"`
	ItemID string             `json:"item_id,omitempty"`
	Item   *storage.VaultItem `json:"item,omitempty"`
}

func AddMCPTools(server *mcp.Server, service *Service, browser BrowserFiller, broker InteractionBroker) {
	if server == nil || service == nil {
		return
	}
	tools := mcpTools{service: service, browser: browser, broker: broker}
	readOnly := true
	destructive := true
	openWorld := false
	mcp.AddTool(server, &mcp.Tool{
		Name:        ToolVaultList,
		Title:       "List personal vault items",
		Description: "List non-secret metadata and opaque item IDs from the user's personal Vault. Never returns passwords, card numbers, CVVs, or TOTP secrets.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: readOnly, OpenWorldHint: &openWorld},
	}, tools.List)
	mcp.AddTool(server, &mcp.Tool{
		Name:        ToolVaultFill,
		Title:       "Blind-autofill vault item into browser",
		Description: "Inject a personal Vault item directly into browser form fields by opaque item_id and element refs without exposing secrets to the model. Logins autofill silently on an exact matching origin; payment cards always pause for explicit per-transaction user approval.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &destructive, OpenWorldHint: &openWorld},
	}, tools.Fill)
	mcp.AddTool(server, &mcp.Tool{
		Name:        ToolVaultRequestItem,
		Title:       "Request new vault item via secure form",
		Description: "Open an inline Secure Form card in the current thread so the user can save a missing login, payment card, or identity directly into the encrypted Vault without putting secrets into chat history. Returns only the new opaque item_id and non-secret metadata.",
		Annotations: &mcp.ToolAnnotations{OpenWorldHint: &openWorld},
	}, tools.RequestItem)
}

func RemoveMCPTools(server *mcp.Server) {
	if server != nil {
		server.RemoveTools(mcpToolNames...)
	}
}

type mcpTools struct {
	service *Service
	browser BrowserFiller
	broker  InteractionBroker
}

func (t mcpTools) List(ctx context.Context, _ *mcp.CallToolRequest, input ListInput) (*mcp.CallToolResult, ListOutput, error) {
	items, err := t.service.List(ctx, input.Kind, input.Domain)
	if err != nil {
		return nil, ListOutput{}, err
	}
	return nil, ListOutput{Items: items}, nil
}

func (t mcpTools) Fill(ctx context.Context, req *mcp.CallToolRequest, input FillInput) (*mcp.CallToolResult, FillOutput, error) {
	if t.browser == nil {
		return nil, FillOutput{}, errors.New("browser backend is not connected; open this conversation in the Jaz desktop app")
	}
	if len(input.Fields) == 0 {
		return nil, FillOutput{}, fmt.Errorf("%w: fields map must include at least one slot", ErrInvalidInput)
	}
	expectedOrigin, err := NormalizeOrigin(input.ExpectedOrigin)
	if err != nil {
		return nil, FillOutput{}, err
	}
	sessionID := mcpsession.SessionID(req)
	liveOrigin, err := t.browser.ActivePageOrigin(ctx, sessionID)
	if err != nil {
		return nil, FillOutput{}, err
	}
	normLive, err := NormalizeOrigin(liveOrigin)
	if err != nil || normLive != expectedOrigin {
		return nil, FillOutput{}, fmt.Errorf("%w: active tab origin %q does not match expected_origin %q", ErrOriginMismatch, liveOrigin, expectedOrigin)
	}

	item, err := t.service.Get(ctx, input.ItemID)
	if err != nil {
		return nil, FillOutput{}, err
	}
	if item.Kind == storage.VaultKindLogin && !OriginMatchesLogin(item.Origins, expectedOrigin) {
		return nil, FillOutput{}, fmt.Errorf("%w: login %s is not authorized for %s", ErrOriginMismatch, item.ID, expectedOrigin)
	}

	if item.Kind == storage.VaultKindCard {
		if t.broker == nil || sessionID == "" {
			return nil, FillOutput{}, ErrSessionRequired
		}
		approved, err := t.broker.ConfirmVaultFill(ctx, sessionID, sessionevents.VaultApprovalSpec{
			ItemID:          item.ID,
			Kind:            item.Kind,
			Label:           item.Label,
			CardBrand:       item.CardBrand,
			CardLast4:       item.CardLast4,
			CardExpMonth:    item.CardExpMonth,
			CardExpYear:     item.CardExpYear,
			MerchantOrigin:  expectedOrigin,
			CheckoutSummary: strings.TrimSpace(input.CheckoutSummary),
		})
		if err != nil {
			return nil, FillOutput{}, err
		}
		if !approved {
			return nil, FillOutput{Status: "denied", Reason: "User denied card autofill."}, nil
		}
	}

	slots := make([]string, 0, len(input.Fields))
	for slot := range input.Fields {
		slots = append(slots, slot)
	}
	slices.Sort(slots)

	_, values, err := t.service.ResolveFillValues(ctx, item.ID, expectedOrigin, slots)
	if err != nil {
		return nil, FillOutput{}, err
	}
	fields := make([]BlindFillField, 0, len(slots))
	for _, slot := range slots {
		normSlot := strings.ToLower(strings.TrimSpace(slot))
		ref := strings.TrimSpace(input.Fields[slot])
		if ref == "" {
			return nil, FillOutput{}, fmt.Errorf("%w: empty element ref for slot %q", ErrInvalidInput, slot)
		}
		fields = append(fields, BlindFillField{
			Slot:  normSlot,
			Ref:   ref,
			Value: values[normSlot],
		})
	}
	defer func() {
		for i := range fields {
			fields[i].Value = ""
		}
		clear(values)
	}()

	res, err := t.browser.BlindFill(ctx, sessionID, BlindFillRequest{
		ExpectedOrigin:      expectedOrigin,
		AllowPaymentIframes: item.Kind == storage.VaultKindCard,
		Fields:              fields,
	})
	if err != nil {
		return nil, FillOutput{}, err
	}
	return nil, FillOutput{
		Status:      "filled",
		FilledSlots: res.FilledSlots,
	}, nil
}

func (t mcpTools) RequestItem(ctx context.Context, req *mcp.CallToolRequest, input RequestItemInput) (*mcp.CallToolResult, RequestItemOutput, error) {
	kind, err := NormalizeKind(input.Kind)
	if err != nil {
		return nil, RequestItemOutput{}, err
	}
	sessionID := mcpsession.SessionID(req)
	if t.broker == nil || sessionID == "" {
		return nil, RequestItemOutput{}, ErrSessionRequired
	}
	domain := strings.TrimSpace(input.Domain)
	if domain != "" && kind == storage.VaultKindLogin {
		if norm, err := NormalizeOrigin(domain); err == nil {
			domain = norm
		}
	}
	itemID, cancelled, err := t.broker.RequestVaultItem(ctx, sessionID, sessionevents.VaultRequestSpec{
		Kind:   kind,
		Domain: domain,
		Label:  strings.TrimSpace(input.Label),
		Reason: strings.TrimSpace(input.Reason),
	})
	if err != nil {
		return nil, RequestItemOutput{}, err
	}
	if cancelled || strings.TrimSpace(itemID) == "" {
		return nil, RequestItemOutput{Status: "cancelled"}, nil
	}
	item, err := t.service.Get(ctx, itemID)
	if err != nil {
		return nil, RequestItemOutput{}, err
	}
	return nil, RequestItemOutput{
		Status: "created",
		ItemID: item.ID,
		Item:   &item,
	}, nil
}
