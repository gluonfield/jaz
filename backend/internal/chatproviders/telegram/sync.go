package telegram

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/gotd/td/tg"
	"github.com/wins/jaz/backend/pkg/integrations"
)

func (p *Provider) dispatcherForSession(session *qrSession) tg.UpdateDispatcher {
	dispatcher := tg.NewUpdateDispatcher()
	dispatcher.OnNewMessage(func(ctx context.Context, e tg.Entities, update *tg.UpdateNewMessage) error {
		connection, ok := session.connectionSnapshot()
		if !ok {
			return nil
		}
		return p.writeTelegramMessage(ctx, connection, update.Message)
	})
	dispatcher.OnNewChannelMessage(func(ctx context.Context, e tg.Entities, update *tg.UpdateNewChannelMessage) error {
		connection, ok := session.connectionSnapshot()
		if !ok {
			return nil
		}
		return p.writeTelegramMessage(ctx, connection, update.Message)
	})
	return dispatcher
}

func (p *Provider) dispatcherForConnection(connection integrations.Connection) tg.UpdateDispatcher {
	dispatcher := tg.NewUpdateDispatcher()
	dispatcher.OnNewMessage(func(ctx context.Context, e tg.Entities, update *tg.UpdateNewMessage) error {
		return p.writeTelegramMessage(ctx, connection, update.Message)
	})
	dispatcher.OnNewChannelMessage(func(ctx context.Context, e tg.Entities, update *tg.UpdateNewChannelMessage) error {
		return p.writeTelegramMessage(ctx, connection, update.Message)
	})
	return dispatcher
}

func (p *Provider) writeContacts(ctx context.Context, connection integrations.Connection, api *tg.Client) error {
	if p.raw == nil {
		return nil
	}
	if p.contactsSyncSuppressed(connection.ID) {
		return nil
	}
	var result tg.ContactsContactsClass
	err := p.foregroundCall(ctx, func(ctx context.Context) error {
		var err error
		result, err = api.ContactsGetContacts(ctx, 0)
		return err
	})
	if err != nil {
		var flood floodWaitError
		if errors.As(err, &flood) {
			if markErr := p.markContactsSyncSuppressed(connection.ID); markErr != nil {
				return errors.Join(err, markErr)
			}
		}
		return err
	}
	contacts, ok := result.(*tg.ContactsContacts)
	if !ok {
		return nil
	}
	records := make([]integrations.Record, 0, len(contacts.Users))
	for _, item := range contacts.Users {
		user, ok := item.(*tg.User)
		if !ok {
			continue
		}
		records = append(records, telegramContactRecord(connection, user))
	}
	if err := p.raw.WriteRecords(ctx, records); err != nil {
		return err
	}
	return p.markContactsSyncSuppressed(connection.ID)
}

func (p *Provider) exportHistory(ctx context.Context, connection integrations.Connection, api *tg.Client) error {
	if p.raw == nil {
		return nil
	}
	p.backfillMu.Lock()
	defer p.backfillMu.Unlock()

	state, err := p.loadBackfillState(connection.ID)
	if err != nil {
		return err
	}
	if state.Completed || p.backfillComplete(connection.ID) {
		return nil
	}
	if !state.PausedUntil.IsZero() && time.Now().UTC().Before(state.PausedUntil) {
		return nil
	}
	budget := newBackfillBudget()
	for {
		page, err := p.getDialogsPage(ctx, api, budget, state.DialogOffset)
		if err != nil {
			return p.handleBackfillPause(connection.ID, state, err)
		}
		modified, ok := page.AsModified()
		if !ok {
			break
		}
		dialogs := modified.GetDialogs()
		if len(dialogs) == 0 {
			break
		}
		users, chats, channels := peerMaps(modified.GetUsers(), modified.GetChats())
		if err := p.raw.WriteRecords(ctx, telegramPeerRecords(connection, users, chats, channels)); err != nil {
			return err
		}
		if err := p.writeMessages(ctx, connection, modified.GetMessages()); err != nil {
			return err
		}
		for _, dialog := range dialogs {
			ref, ok := peerRefFromDialog(dialog, users, chats, channels)
			if !ok {
				continue
			}
			if state.CompletedPeers[ref.key()] {
				continue
			}
			offsetID := 0
			if state.CurrentPeer != nil && state.CurrentPeer.key() == ref.key() {
				offsetID = state.CurrentPeerOffsetID
			}
			state.CurrentPeer = &ref
			state.CurrentPeerOffsetID = offsetID
			if err := p.saveBackfillState(connection.ID, state); err != nil {
				return err
			}
			done, nextOffsetID, err := p.exportDialogHistory(ctx, connection, api, budget, ref.inputPeer(), offsetID)
			if err != nil {
				state.CurrentPeerOffsetID = nextOffsetID
				return p.handleBackfillPause(connection.ID, state, err)
			}
			state.CurrentPeerOffsetID = nextOffsetID
			if !done {
				return p.saveBackfillState(connection.ID, state)
			}
			state.CompletedPeers[ref.key()] = true
			state.CurrentPeer = nil
			state.CurrentPeerOffsetID = 0
			if err := p.saveBackfillState(connection.ID, state); err != nil {
				return err
			}
		}
		last := dialogs[len(dialogs)-1]
		nextPeer, ok := peerRefFromDialog(last, users, chats, channels)
		if !ok {
			break
		}
		nextOffsetID := last.GetTopMessage()
		if nextOffsetID == 0 || nextOffsetID == state.DialogOffset.ID {
			break
		}
		state.DialogOffset = dialogOffset{
			Peer: &nextPeer,
			ID:   nextOffsetID,
			Date: messageDate(modified.GetMessages(), nextOffsetID),
		}
		if err := p.saveBackfillState(connection.ID, state); err != nil {
			return err
		}
	}
	return p.finishBackfill(connection.ID, state)
}

func (p *Provider) startBackfillLoop(ctx context.Context, connection integrations.Connection, api *tg.Client) {
	go func() {
		for {
			if p.backfillComplete(connection.ID) {
				return
			}
			_ = p.exportHistory(ctx, connection, api)
			timer := time.NewTimer(telegramBackfillRunInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
}

func (p *Provider) exportDialogHistory(ctx context.Context, connection integrations.Connection, api *tg.Client, budget *backfillBudget, peer tg.InputPeerClass, offsetID int) (bool, int, error) {
	for {
		history, err := p.getHistoryPage(ctx, api, budget, peer, offsetID)
		if err != nil {
			return false, offsetID, err
		}
		modified, ok := history.AsModified()
		if !ok {
			return true, offsetID, nil
		}
		messages := modified.GetMessages()
		if len(messages) == 0 {
			return true, offsetID, nil
		}
		users, chats, channels := peerMaps(modified.GetUsers(), modified.GetChats())
		if err := p.raw.WriteRecords(ctx, telegramPeerRecords(connection, users, chats, channels)); err != nil {
			return false, offsetID, err
		}
		if err := p.writeMessages(ctx, connection, messages); err != nil {
			return false, offsetID, err
		}
		nextOffsetID := lowestMessageID(messages)
		if nextOffsetID == 0 || nextOffsetID == offsetID {
			return true, offsetID, nil
		}
		offsetID = nextOffsetID
	}
}

func (p *Provider) getDialogsPage(ctx context.Context, api *tg.Client, budget *backfillBudget, offset dialogOffset) (tg.MessagesDialogsClass, error) {
	var page tg.MessagesDialogsClass
	err := p.backfillCall(ctx, budget, func(ctx context.Context) error {
		var err error
		page, err = api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
			OffsetPeer: offset.inputPeer(),
			OffsetID:   offset.ID,
			OffsetDate: offset.Date,
			Limit:      telegramBackfillPageLimit,
		})
		return err
	})
	return page, err
}

func (p *Provider) getHistoryPage(ctx context.Context, api *tg.Client, budget *backfillBudget, peer tg.InputPeerClass, offsetID int) (tg.MessagesMessagesClass, error) {
	var history tg.MessagesMessagesClass
	err := p.backfillCall(ctx, budget, func(ctx context.Context) error {
		var err error
		history, err = api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
			Peer:     peer,
			OffsetID: offsetID,
			Limit:    telegramBackfillPageLimit,
		})
		return err
	})
	return history, err
}

func (p *Provider) handleBackfillPause(connectionID string, state backfillState, err error) error {
	if errors.Is(err, errBackfillBudget) {
		state.PausedUntil = time.Now().UTC().Add(telegramBackfillRunInterval)
		return p.saveBackfillState(connectionID, state)
	}
	var flood floodWaitError
	if errors.As(err, &flood) {
		state.PausedUntil = time.Now().UTC().Add(flood.wait)
		return p.saveBackfillState(connectionID, state)
	}
	return err
}

func (p *Provider) writeTelegramMessage(ctx context.Context, connection integrations.Connection, message tg.MessageClass) error {
	if p.raw == nil {
		return nil
	}
	records := telegramMessageRecords(connection, []tg.MessageClass{message})
	if len(records) == 0 {
		return nil
	}
	return p.raw.WriteRecords(ctx, records)
}

func (p *Provider) writeMessages(ctx context.Context, connection integrations.Connection, messages []tg.MessageClass) error {
	records := telegramMessageRecords(connection, messages)
	if len(records) == 0 {
		return nil
	}
	return p.raw.WriteRecords(ctx, records)
}

func (p *Provider) contactsSyncSuppressed(connectionID string) bool {
	data, err := os.ReadFile(p.contactsMarkerPath(connectionID))
	if err != nil {
		return false
	}
	last, err := time.Parse(time.RFC3339Nano, string(data))
	if err != nil {
		return false
	}
	return time.Since(last) < telegramContactsSyncInterval
}

func (p *Provider) markContactsSyncSuppressed(connectionID string) error {
	return os.WriteFile(p.contactsMarkerPath(connectionID), []byte(time.Now().UTC().Format(time.RFC3339Nano)), 0o600)
}
