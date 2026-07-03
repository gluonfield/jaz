package whatsapp

import (
	"context"
	"sort"
	"strings"

	whatsappconnector "github.com/wins/jaz/backend/internal/connectors/whatsapp"
	waTypes "go.mau.fi/whatsmeow/types"
)

func (p *Provider) Search(ctx context.Context, req whatsappconnector.SearchRequest) (whatsappconnector.SearchResult, error) {
	client, err := p.clientForConnection(ctx, req.Connection)
	if err != nil {
		return whatsappconnector.SearchResult{}, err
	}
	contacts, err := client.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		return whatsappconnector.SearchResult{}, err
	}
	query := strings.ToLower(strings.TrimSpace(req.Query))
	limit := whatsappconnector.SearchLimit(req.Limit)
	items := make([]whatsappconnector.SearchItem, 0, len(contacts))
	for jid, contact := range contacts {
		item := whatsappSearchItem(jid, contact)
		if query != "" && !whatsappSearchMatch(item, query) {
			continue
		}
		items = append(items, item)
	}
	for _, item := range whatsappGroupSearchItems(ctx, client) {
		if query != "" && !whatsappSearchMatch(item, query) {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return whatsappconnector.SearchResult{Items: items}, nil
}

func whatsappSearchItem(jid waTypes.JID, contact waTypes.ContactInfo) whatsappconnector.SearchItem {
	names := contactNames(contact)
	display := whatsappContactDisplayName(jid, contact, names)
	item := whatsappconnector.SearchItem{
		Kind:  whatsappSearchKind(jid),
		Name:  display,
		Phone: whatsappSearchPhone(jid),
		JID:   jid.String(),
	}
	if item.Name == "" {
		item.Name = jid.String()
	}
	return item
}

func whatsappSearchKind(jid waTypes.JID) whatsappconnector.SearchItemKind {
	switch jid.Server {
	case waTypes.GroupServer:
		return whatsappconnector.SearchItemGroup
	case waTypes.BroadcastServer:
		return whatsappconnector.SearchItemBroadcast
	default:
		return whatsappconnector.SearchItemPerson
	}
}

func whatsappSearchPhone(jid waTypes.JID) string {
	switch jid.Server {
	case waTypes.DefaultUserServer, waTypes.LegacyUserServer:
		return jid.User
	default:
		return ""
	}
}

func whatsappSearchMatch(item whatsappconnector.SearchItem, query string) bool {
	candidates := []struct {
		value string
	}{
		{value: item.Name},
		{value: item.Phone},
		{value: item.JID},
	}
	for _, candidate := range candidates {
		if candidate.value != "" && strings.Contains(strings.ToLower(candidate.value), query) {
			return true
		}
	}
	if phone := digits(query); phone != "" && strings.Contains(item.Phone, phone) {
		return true
	}
	return false
}
