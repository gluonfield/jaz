package telegram

import (
	"context"
	"strconv"
	"strings"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	telegramconnector "github.com/wins/jaz/backend/internal/connectors/telegram"
)

const telegramSearchDialogLimit = 100

func (p *Provider) Search(ctx context.Context, req telegramconnector.SearchRequest) (telegramconnector.SearchResult, error) {
	query := strings.TrimSpace(req.Query)
	limit := telegramconnector.SearchLimit(req.Limit)
	var items []telegramconnector.SearchItem
	search := func(runCtx context.Context, client *telegram.Client) error {
		found, err := p.searchTelegram(runCtx, client.API(), query, limit)
		if err != nil {
			return err
		}
		items = found
		return nil
	}
	if client := p.client(req.Connection.ID); client != nil {
		if err := search(ctx, client); err != nil {
			return telegramconnector.SearchResult{}, err
		}
		return telegramconnector.SearchResult{Items: items}, nil
	}
	if err := p.withClient(ctx, req.Connection.ID, true, search); err != nil {
		return telegramconnector.SearchResult{}, err
	}
	return telegramconnector.SearchResult{Items: items}, nil
}

func (p *Provider) searchTelegram(ctx context.Context, api *tg.Client, query string, limit int) ([]telegramconnector.SearchItem, error) {
	results := telegramSearchResults{seen: map[string]bool{}}
	if query == "" {
		if err := p.addTelegramContacts(ctx, api, &results, limit); err != nil {
			return nil, err
		}
	} else if err := p.addTelegramSearchResults(ctx, api, query, &results, limit); err != nil {
		return nil, err
	}
	if len(results.items) < limit {
		if err := p.addTelegramDialogs(ctx, api, query, &results, limit); err != nil {
			return nil, err
		}
	}
	return results.items, nil
}

func (p *Provider) addTelegramContacts(ctx context.Context, api *tg.Client, results *telegramSearchResults, limit int) error {
	var contacts tg.ContactsContactsClass
	err := p.foregroundCall(ctx, func(ctx context.Context) error {
		var err error
		contacts, err = api.ContactsGetContacts(ctx, 0)
		return err
	})
	if err != nil {
		return err
	}
	modified, ok := contacts.AsModified()
	if !ok {
		return nil
	}
	for _, item := range modified.Users {
		user, ok := item.(*tg.User)
		if !ok {
			continue
		}
		if searchItem, ok := telegramUserSearchItem(user); ok {
			results.add(searchItem, limit)
		}
	}
	return nil
}

func (p *Provider) addTelegramSearchResults(ctx context.Context, api *tg.Client, query string, results *telegramSearchResults, limit int) error {
	var found *tg.ContactsFound
	err := p.foregroundCall(ctx, func(ctx context.Context) error {
		var err error
		found, err = api.ContactsSearch(ctx, &tg.ContactsSearchRequest{Q: query, Limit: limit})
		return err
	})
	if err != nil {
		return err
	}
	users, chats, channels := peerMaps(found.Users, found.Chats)
	for _, peer := range append(found.MyResults, found.Results...) {
		if item, ok := telegramPeerSearchItem(peer, users, chats, channels); ok {
			results.add(item, limit)
		}
	}
	for _, user := range found.Users {
		if item, ok := telegramUserSearchItem(asTelegramUser(user)); ok {
			results.add(item, limit)
		}
	}
	for _, chat := range found.Chats {
		switch item := chat.(type) {
		case *tg.Chat:
			results.add(telegramChatSearchItem(item), limit)
		case *tg.Channel:
			if searchItem, ok := telegramChannelSearchItem(item); ok {
				results.add(searchItem, limit)
			}
		}
	}
	return nil
}

func (p *Provider) addTelegramDialogs(ctx context.Context, api *tg.Client, query string, results *telegramSearchResults, limit int) error {
	var dialogs tg.MessagesDialogsClass
	err := p.foregroundCall(ctx, func(ctx context.Context) error {
		var err error
		dialogs, err = api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
			OffsetPeer: &tg.InputPeerEmpty{},
			Limit:      telegramSearchDialogLimit,
		})
		return err
	})
	if err != nil {
		return err
	}
	modified, ok := dialogs.AsModified()
	if !ok {
		return nil
	}
	users, chats, channels := peerMaps(modified.GetUsers(), modified.GetChats())
	for _, dialog := range modified.GetDialogs() {
		if len(results.items) >= limit {
			return nil
		}
		item, ok := telegramPeerSearchItem(dialog.GetPeer(), users, chats, channels)
		if !ok {
			continue
		}
		if query != "" && !chatSearchItemMatch(item, query) {
			continue
		}
		results.add(item, limit)
	}
	return nil
}

type telegramSearchResults struct {
	items []telegramconnector.SearchItem
	seen  map[string]bool
}

func (r *telegramSearchResults) add(item telegramconnector.SearchItem, limit int) {
	if item.Recipient == "" || r.seen[item.Recipient] || len(r.items) >= limit {
		return
	}
	r.seen[item.Recipient] = true
	r.items = append(r.items, item)
}

func telegramPeerSearchItem(peer tg.PeerClass, users map[int64]*tg.User, chats map[int64]*tg.Chat, channels map[int64]*tg.Channel) (telegramconnector.SearchItem, bool) {
	switch p := peer.(type) {
	case *tg.PeerUser:
		return telegramUserSearchItem(users[p.UserID])
	case *tg.PeerChat:
		chat := chats[p.ChatID]
		if chat == nil {
			return telegramconnector.SearchItem{}, false
		}
		return telegramChatSearchItem(chat), true
	case *tg.PeerChannel:
		return telegramChannelSearchItem(channels[p.ChannelID])
	default:
		return telegramconnector.SearchItem{}, false
	}
}

func asTelegramUser(user tg.UserClass) *tg.User {
	item, ok := user.(*tg.User)
	if !ok {
		return nil
	}
	return item
}

func telegramUserSearchItem(user *tg.User) (telegramconnector.SearchItem, bool) {
	if user == nil {
		return telegramconnector.SearchItem{}, false
	}
	username := telegramUsername(user.Username)
	recipient := username
	if hash, ok := user.GetAccessHash(); ok {
		recipient = "user:" + strconv.FormatInt(user.ID, 10) + ":" + strconv.FormatInt(hash, 10)
	}
	if recipient == "" {
		return telegramconnector.SearchItem{}, false
	}
	kind := telegramconnector.SearchItemPerson
	if user.Bot {
		kind = telegramconnector.SearchItemBot
	}
	return telegramconnector.SearchItem{
		Kind:      kind,
		Name:      telegramUserName(user),
		Username:  username,
		Phone:     user.Phone,
		Recipient: recipient,
		PeerID:    "user:" + strconv.FormatInt(user.ID, 10),
	}, true
}

func telegramChatSearchItem(chat *tg.Chat) telegramconnector.SearchItem {
	id := strconv.FormatInt(chat.ID, 10)
	return telegramconnector.SearchItem{
		Kind:      telegramconnector.SearchItemGroup,
		Name:      chat.Title,
		Recipient: "chat:" + id,
		PeerID:    "chat:" + id,
	}
}

func telegramChannelSearchItem(channel *tg.Channel) (telegramconnector.SearchItem, bool) {
	if channel == nil {
		return telegramconnector.SearchItem{}, false
	}
	hash, ok := channel.GetAccessHash()
	if !ok {
		return telegramconnector.SearchItem{}, false
	}
	id := strconv.FormatInt(channel.ID, 10)
	kind := telegramconnector.SearchItemChannel
	if channel.Megagroup {
		kind = telegramconnector.SearchItemGroup
	}
	return telegramconnector.SearchItem{
		Kind:      kind,
		Name:      channel.Title,
		Username:  telegramUsername(channel.Username),
		Recipient: "channel:" + id + ":" + strconv.FormatInt(hash, 10),
		PeerID:    "channel:" + id,
	}, true
}

func telegramUsername(username string) string {
	username = strings.TrimSpace(username)
	if username == "" {
		return ""
	}
	return "@" + strings.TrimPrefix(username, "@")
}

func chatSearchItemMatch(item telegramconnector.SearchItem, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	for _, candidate := range []string{
		item.Name,
		item.Username,
		item.Phone,
		item.Recipient,
		item.PeerID,
	} {
		if candidate != "" && strings.Contains(strings.ToLower(candidate), query) {
			return true
		}
	}
	return false
}
