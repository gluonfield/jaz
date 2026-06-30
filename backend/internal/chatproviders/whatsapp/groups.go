package whatsapp

import (
	"context"

	whatsappconnector "github.com/wins/jaz/backend/internal/connectors/whatsapp"
	"github.com/wins/jaz/backend/pkg/integrations"
	"go.mau.fi/whatsmeow"
	waTypes "go.mau.fi/whatsmeow/types"
)

func (p *Provider) writeAllGroups(ctx context.Context, connection integrations.Connection, client *whatsmeow.Client) error {
	groups, err := client.GetJoinedGroups(ctx)
	if err != nil {
		return err
	}
	records := make([]integrations.Record, 0, len(groups))
	for _, group := range groups {
		records = append(records, whatsappGroupRecord(connection, group.JID, group.Name))
	}
	return p.writeRecords(ctx, records...)
}

func whatsappGroupRecord(connection integrations.Connection, jid waTypes.JID, name string) integrations.Record {
	var names []string
	if name != "" {
		names = []string{name}
	}
	raw := rawJSON(map[string]any{
		"whatsapp_id":   jid.String(),
		"jid":           jid.String(),
		"display_name":  firstNonEmpty(name, jid.String()),
		"contact_names": names,
	})
	return integrations.Record{
		Provider:     whatsappconnector.ProviderID,
		ConnectionID: connection.ID,
		AccountID:    connection.AccountID,
		Kind:         "whatsapp.contact",
		ExternalID:   jid.String(),
		Raw:          raw,
	}
}

func whatsappGroupSearchItems(ctx context.Context, client *whatsmeow.Client) []whatsappconnector.SearchItem {
	groups, err := client.GetJoinedGroups(ctx)
	if err != nil {
		return nil
	}
	items := make([]whatsappconnector.SearchItem, 0, len(groups))
	for _, group := range groups {
		items = append(items, whatsappconnector.SearchItem{
			Kind: whatsappconnector.SearchItemGroup,
			Name: firstNonEmpty(group.Name, group.JID.String()),
			JID:  group.JID.String(),
		})
	}
	return items
}
