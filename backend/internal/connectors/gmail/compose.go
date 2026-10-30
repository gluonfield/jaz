package gmail

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"strings"
)

const MaxAttachmentBytes = 25 << 20

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
	if input.BodyText == "" && input.BodyHTML == "" {
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
	body, err := composeBody(input)
	if err != nil {
		return "", err
	}
	for key, values := range body.header {
		message.WriteString(key + ": " + strings.Join(values, ", ") + "\r\n")
	}
	message.WriteString("\r\n")
	message.Write(body.data)
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

type mimeEntity struct {
	header textproto.MIMEHeader
	data   []byte
}

func composeBody(input ComposeMessageRequest) (mimeEntity, error) {
	body, err := textEntity("text/plain", input.BodyText)
	if err != nil {
		return mimeEntity{}, err
	}
	if input.BodyHTML != "" {
		html, err := textEntity("text/html", input.BodyHTML)
		if err != nil {
			return mimeEntity{}, err
		}
		body, err = multipartEntity("alternative", []mimeEntity{body, html})
		if err != nil {
			return mimeEntity{}, err
		}
	}
	if len(input.Attachments) == 0 {
		return body, nil
	}
	parts := []mimeEntity{body}
	total := 0
	for _, attachment := range input.Attachments {
		total += len(attachment.Data)
		if total > MaxAttachmentBytes {
			return mimeEntity{}, fmt.Errorf("attachments exceed the 25 MiB total limit")
		}
		mimeType := attachment.Attachment.MIMEType
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		disposition := "attachment"
		if attachment.Attachment.Inline {
			disposition = "inline"
		}
		header := textproto.MIMEHeader{
			"Content-Type":              {mimeType},
			"Content-Disposition":       {mime.FormatMediaType(disposition, map[string]string{"filename": attachment.Attachment.FileName})},
			"Content-Transfer-Encoding": {"base64"},
		}
		if attachment.ContentID != "" {
			header.Set("Content-ID", "<"+strings.Trim(cleanHeader(attachment.ContentID), "<>")+">")
		}
		encoded := base64.StdEncoding.EncodeToString(attachment.Data)
		var data bytes.Buffer
		for len(encoded) > 0 {
			n := min(76, len(encoded))
			data.WriteString(encoded[:n] + "\r\n")
			encoded = encoded[n:]
		}
		parts = append(parts, mimeEntity{header: header, data: data.Bytes()})
	}
	return multipartEntity("mixed", parts)
}

func textEntity(contentType, text string) (mimeEntity, error) {
	var data bytes.Buffer
	body := quotedprintable.NewWriter(&data)
	if _, err := body.Write([]byte(text)); err != nil {
		return mimeEntity{}, err
	}
	if err := body.Close(); err != nil {
		return mimeEntity{}, err
	}
	return mimeEntity{
		header: textproto.MIMEHeader{"Content-Type": {contentType + "; charset=UTF-8"}, "Content-Transfer-Encoding": {"quoted-printable"}},
		data:   data.Bytes(),
	}, nil
}

func multipartEntity(subtype string, parts []mimeEntity) (mimeEntity, error) {
	var data bytes.Buffer
	writer := multipart.NewWriter(&data)
	for _, part := range parts {
		out, err := writer.CreatePart(part.header)
		if err != nil {
			return mimeEntity{}, err
		}
		if _, err := out.Write(part.data); err != nil {
			return mimeEntity{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return mimeEntity{}, err
	}
	return mimeEntity{
		header: textproto.MIMEHeader{"Content-Type": {mime.FormatMediaType("multipart/"+subtype, map[string]string{"boundary": writer.Boundary()})}},
		data:   data.Bytes(),
	}, nil
}
