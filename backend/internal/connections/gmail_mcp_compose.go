package connections

import (
	"errors"
	"net/mail"
	"slices"
	"strings"

	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
)

func gmailCreateDraftRequest(input GmailCreateDraftInput) (gmailconnector.ComposeMessageRequest, error) {
	return gmailValidateCompose(gmailconnector.ComposeMessageRequest{
		To:       cleanGmailAddressList(input.To),
		Cc:       cleanGmailAddressList(input.Cc),
		Bcc:      cleanGmailAddressList(input.Bcc),
		Subject:  strings.TrimSpace(input.Subject),
		BodyText: input.BodyText,
	})
}

func gmailUpdateDraftRequest(input GmailUpdateDraftInput, current gmailconnector.DraftContent) (gmailconnector.ComposeMessageRequest, error) {
	request := gmailconnector.ComposeMessageRequest{
		ThreadID:   current.Draft.Message.ThreadID,
		To:         addressValues(current.Draft.Message.To),
		Cc:         addressValues(current.Draft.Message.Cc),
		Bcc:        addressValues(current.Draft.Message.Bcc),
		Subject:    current.Draft.Message.Subject,
		BodyText:   current.BodyText,
		InReplyTo:  current.Draft.Message.InReplyTo,
		References: current.Draft.Message.References,
	}
	if input.To != nil {
		request.To = cleanGmailAddressList(input.To)
	}
	if input.Cc != nil {
		request.Cc = cleanGmailAddressList(input.Cc)
	}
	if input.Bcc != nil {
		request.Bcc = cleanGmailAddressList(input.Bcc)
	}
	if input.Subject != nil {
		request.Subject = strings.TrimSpace(*input.Subject)
	}
	if input.BodyText != nil {
		request.BodyText = *input.BodyText
	}
	return gmailValidateCompose(request)
}

func gmailReplyDraftRequest(input GmailCreateReplyDraftInput, thread gmailconnector.ThreadContent, accountEmail string) (gmailconnector.ComposeMessageRequest, error) {
	target, err := gmailReplyTarget(input, thread)
	if err != nil {
		return gmailconnector.ComposeMessageRequest{}, err
	}
	mode, err := gmailReplyMode(input.ReplyMode)
	if err != nil {
		return gmailconnector.ComposeMessageRequest{}, err
	}
	to, cc := gmailReplyRecipients(target.Message, mode, accountEmail)
	cc = appendRawAddresses(cc, seenEmails(to), ownEmails(accountEmail), input.CcAdd)
	bcc := appendRawAddresses(nil, seenEmails(append(slices.Clone(to), cc...)), ownEmails(accountEmail), input.BccAdd)
	return gmailValidateCompose(gmailconnector.ComposeMessageRequest{
		ThreadID:   thread.ID,
		To:         to,
		Cc:         cc,
		Bcc:        bcc,
		Subject:    gmailReplySubject(target.Message.Subject),
		BodyText:   input.BodyText,
		InReplyTo:  target.Message.MessageID,
		References: gmailReplyReferences(target.Message),
	})
}

func gmailValidateCompose(request gmailconnector.ComposeMessageRequest) (gmailconnector.ComposeMessageRequest, error) {
	if len(request.To) == 0 {
		return gmailconnector.ComposeMessageRequest{}, errors.New("to is required")
	}
	if strings.TrimSpace(request.BodyText) == "" {
		return gmailconnector.ComposeMessageRequest{}, errors.New("body_text is required")
	}
	return request, nil
}

func gmailReplyTarget(input GmailCreateReplyDraftInput, thread gmailconnector.ThreadContent) (gmailconnector.MessageContent, error) {
	if len(thread.Messages) == 0 {
		return gmailconnector.MessageContent{}, errors.New("thread has no messages")
	}
	id := strings.TrimSpace(input.ID)
	idType := gmailThreadIDType(input.IDType)
	if id != "" && idType != gmailconnector.IDTypeThread {
		for _, message := range thread.Messages {
			if message.Message.ID == id {
				return message, nil
			}
		}
	}
	return thread.Messages[len(thread.Messages)-1], nil
}

func gmailReplyMode(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "reply":
		return "reply", nil
	case "reply_all":
		return "reply_all", nil
	default:
		return "", errors.New("reply_mode must be reply or reply_all")
	}
}

func gmailReplyRecipients(message gmailconnector.Message, mode, accountEmail string) ([]string, []string) {
	own := ownEmails(accountEmail)
	toSeen := map[string]struct{}{}
	ccSeen := map[string]struct{}{}
	to := []string{}
	cc := []string{}
	fromIsSelf := anyAddressInSet(message.From, own)
	if fromIsSelf {
		to = appendAddresses(to, toSeen, own, message.To)
	} else if len(message.ReplyTo) > 0 {
		to = appendAddresses(to, toSeen, own, message.ReplyTo)
	} else {
		to = appendAddresses(to, toSeen, own, message.From)
	}
	if mode == "reply_all" {
		to = appendAddresses(to, toSeen, own, message.To)
		for email := range toSeen {
			ccSeen[email] = struct{}{}
		}
		cc = appendAddresses(cc, ccSeen, own, message.Cc)
	}
	return to, cc
}

func gmailReplySubject(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(subject), "re:") {
		return subject
	}
	return "Re: " + subject
}

func gmailReplyReferences(message gmailconnector.Message) string {
	refs := strings.Fields(strings.TrimSpace(message.References))
	if message.MessageID != "" && !slices.Contains(refs, message.MessageID) {
		refs = append(refs, message.MessageID)
	}
	return strings.Join(refs, " ")
}

func addressValues(addresses []gmailconnector.Address) []string {
	out := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if address.Email == "" {
			continue
		}
		if address.Name != "" {
			out = append(out, (&mail.Address{Name: address.Name, Address: address.Email}).String())
			continue
		}
		out = append(out, address.Email)
	}
	return out
}

func appendAddresses(out []string, seen map[string]struct{}, own map[string]struct{}, addresses []gmailconnector.Address) []string {
	for _, address := range addresses {
		email := normalizedEmail(address.Email)
		if email == "" {
			continue
		}
		if _, ok := own[email]; ok {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		out = append(out, addressValue(address))
	}
	return out
}

func appendRawAddresses(out []string, seen map[string]struct{}, own map[string]struct{}, values []string) []string {
	for _, value := range cleanGmailAddressList(values) {
		parsed, err := mail.ParseAddress(value)
		if err != nil {
			out = append(out, value)
			continue
		}
		email := normalizedEmail(parsed.Address)
		if _, ok := own[email]; ok {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		out = append(out, parsed.String())
	}
	return out
}

func addressValue(address gmailconnector.Address) string {
	if address.Name != "" {
		return (&mail.Address{Name: address.Name, Address: address.Email}).String()
	}
	return address.Email
}

func anyAddressInSet(addresses []gmailconnector.Address, set map[string]struct{}) bool {
	for _, address := range addresses {
		if _, ok := set[normalizedEmail(address.Email)]; ok {
			return true
		}
	}
	return false
}

func seenEmails(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range values {
		parsed, err := mail.ParseAddress(value)
		if err == nil {
			out[normalizedEmail(parsed.Address)] = struct{}{}
			continue
		}
		out[normalizedEmail(value)] = struct{}{}
	}
	return out
}

func ownEmails(accountEmail string) map[string]struct{} {
	out := map[string]struct{}{}
	if email := normalizedEmail(accountEmail); email != "" {
		out[email] = struct{}{}
	}
	return out
}

func normalizedEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func cleanGmailAddressList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
