package gmail

import (
	"context"
	"encoding/base64"
	"fmt"
	"mime"
	"strings"
)

func isAttachmentPart(part messagePart) bool {
	headers := headersByName(part.Headers)
	disposition, _, _ := mime.ParseMediaType(headers["content-disposition"])
	return part.Filename != "" || disposition == "attachment" || headers["content-id"] != ""
}

func isBodyPart(part messagePart) bool {
	return !isAttachmentPart(part) && (strings.EqualFold(part.MIMEType, "text/plain") || strings.EqualFold(part.MIMEType, "text/html"))
}

func (c APIClient) draftAttachments(ctx context.Context, messageID string, part *messagePart) ([]AttachmentContent, error) {
	var attachments []AttachmentContent
	if len(part.Parts) > 0 && part.Body.AttachmentID == "" && part.Body.Data == "" {
		if isAttachmentPart(*part) {
			return nil, fmt.Errorf("cannot preserve %s attachment container without downloadable content", part.MIMEType)
		}
		for i := range part.Parts {
			children, err := c.draftAttachments(ctx, messageID, &part.Parts[i])
			if err != nil {
				return nil, err
			}
			attachments = append(attachments, children...)
		}
		return attachments, nil
	}
	attachment := AttachmentContent{
		Attachment: Attachment{ID: part.Body.AttachmentID, FileName: part.Filename, MIMEType: part.MIMEType, Inline: inlinePart(*part)},
		ContentID:  headersByName(part.Headers)["content-id"],
	}
	var err error
	if attachment.Attachment.ID != "" {
		content, readErr := c.readAttachment(ctx, messageID, attachment.Attachment)
		err = readErr
		attachment.Data = content.Data
	} else {
		attachment.Data, err = decodeBodyBytes(part.Body.Data)
	}
	if err != nil {
		return nil, fmt.Errorf("read draft MIME part %q: %w", part.Filename, err)
	}
	if isBodyPart(*part) {
		part.Body.Data = base64.RawURLEncoding.EncodeToString(attachment.Data)
		return nil, nil
	}
	attachment.Size = int64(len(attachment.Data))
	attachment.Attachment.Size = attachment.Size
	return []AttachmentContent{attachment}, nil
}
