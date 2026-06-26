package connections

import (
	"errors"
	"net/mail"
	"strings"

	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
)

func gmailCreateDraftRequest(input GmailCreateDraftInput) (gmailconnector.ComposeMessageRequest, error) {
	return gmailValidateCompose(gmailconnector.ComposeMessageRequest{
		ThreadID:   strings.TrimSpace(input.ThreadID),
		To:         cleanGmailAddressList(input.To),
		Cc:         cleanGmailAddressList(input.Cc),
		Bcc:        cleanGmailAddressList(input.Bcc),
		Subject:    strings.TrimSpace(input.Subject),
		BodyText:   input.BodyText,
		InReplyTo:  strings.TrimSpace(input.InReplyTo),
		References: strings.TrimSpace(input.References),
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
	if input.ThreadID != nil {
		request.ThreadID = strings.TrimSpace(*input.ThreadID)
	}
	if input.InReplyTo != nil {
		request.InReplyTo = strings.TrimSpace(*input.InReplyTo)
	}
	if input.References != nil {
		request.References = strings.TrimSpace(*input.References)
	}
	return gmailValidateCompose(request)
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
