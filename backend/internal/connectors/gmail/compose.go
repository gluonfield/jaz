package gmail

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"strings"
)

func draftRequest(id string, input ComposeMessageRequest) (apiDraftRequest, error) {
	raw, err := encodeMessage(input)
	if err != nil {
		return apiDraftRequest{}, err
	}
	return apiDraftRequest{
		ID: id,
		Message: apiRawMessage{
			Raw:      raw,
			ThreadID: input.ThreadID,
		},
	}, nil
}

func encodeMessage(input ComposeMessageRequest) (string, error) {
	to, err := formatAddressHeader(input.To)
	if err != nil {
		return "", fmt.Errorf("to: %w", err)
	}
	if to == "" {
		return "", fmt.Errorf("to is required")
	}
	cc, err := formatAddressHeader(input.Cc)
	if err != nil {
		return "", fmt.Errorf("cc: %w", err)
	}
	bcc, err := formatAddressHeader(input.Bcc)
	if err != nil {
		return "", fmt.Errorf("bcc: %w", err)
	}
	if input.BodyText == "" {
		return "", fmt.Errorf("body text is required")
	}
	var message bytes.Buffer
	message.WriteString("To: " + to + "\r\n")
	if cc != "" {
		message.WriteString("Cc: " + cc + "\r\n")
	}
	if bcc != "" {
		message.WriteString("Bcc: " + bcc + "\r\n")
	}
	if input.Subject != "" {
		message.WriteString("Subject: " + mime.QEncoding.Encode("utf-8", input.Subject) + "\r\n")
	}
	if input.InReplyTo != "" {
		message.WriteString("In-Reply-To: " + cleanHeader(input.InReplyTo) + "\r\n")
	}
	if input.References != "" {
		message.WriteString("References: " + cleanHeader(input.References) + "\r\n")
	}
	message.WriteString("MIME-Version: 1.0\r\n")
	message.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	message.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	body := quotedprintable.NewWriter(&message)
	if _, err := body.Write([]byte(input.BodyText)); err != nil {
		return "", err
	}
	if err := body.Close(); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(message.Bytes()), nil
}

func formatAddressHeader(values []string) (string, error) {
	if len(values) == 0 {
		return "", nil
	}
	parsed, err := mail.ParseAddressList(strings.Join(values, ","))
	if err != nil {
		return "", err
	}
	out := make([]string, 0, len(parsed))
	for _, address := range parsed {
		out = append(out, address.String())
	}
	return strings.Join(out, ", "), nil
}

func cleanHeader(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
