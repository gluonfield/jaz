package connections

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gmailconnector "github.com/wins/jaz/backend/internal/connectors/gmail"
	"github.com/wins/jaz/backend/internal/emailclean"
	"github.com/wins/jaz/backend/internal/integrationingest"
	"github.com/wins/jaz/backend/pkg/integrations"
)

const maxGmailAttachmentTextPreviewChars = 1200

func (t *GmailMCPTools) ReadAttachment(ctx context.Context, _ *mcp.CallToolRequest, input GmailReadAttachmentInput) (*mcp.CallToolResult, GmailReadAttachmentOutput, error) {
	messageID := strings.TrimSpace(input.MessageID)
	attachmentID := strings.TrimSpace(input.AttachmentID)
	account := input.Account
	ref, hasRef, err := gmailconnector.ParseAttachmentSourceRef(attachmentID)
	if err != nil {
		return nil, GmailReadAttachmentOutput{}, err
	}
	if hasRef {
		if messageID != "" && messageID != ref.MessageID {
			return nil, GmailReadAttachmentOutput{}, errors.New("message_id does not match attachment ref")
		}
		messageID = ref.MessageID
		attachmentID = ""
		if strings.TrimSpace(account) == "" {
			account = ref.Account
		}
	}
	if messageID == "" {
		return nil, GmailReadAttachmentOutput{}, errors.New("message_id is required")
	}
	if attachmentID == "" && !hasRef {
		return nil, GmailReadAttachmentOutput{}, errors.New("attachment_id is required")
	}
	session, connected, err := t.session(ctx, account)
	if err != nil {
		return nil, GmailReadAttachmentOutput{}, err
	}
	if !connected {
		accountRequired := len(session.accounts) > 1
		out := GmailReadAttachmentOutput{Connected: accountRequired, Accounts: session.accounts, AccountRequired: accountRequired}
		if out.AccountRequired {
			return textResult(gmailAccountRequiredText(session.accounts)), out, nil
		}
		return textResult("Gmail is not connected. Connect Gmail in Settings > Connections."), out, nil
	}
	if hasRef && integrations.NormalizeAlias(session.connection.AccountID) != integrations.NormalizeAlias(ref.Account) {
		return nil, GmailReadAttachmentOutput{}, errors.New("account does not match attachment ref")
	}
	var attachment gmailconnector.AttachmentContent
	if hasRef {
		attachment, err = session.api.ReadAttachmentAt(ctx, messageID, ref.Index)
	} else {
		attachment, err = session.api.ReadAttachment(ctx, messageID, attachmentID)
	}
	if err != nil {
		return nil, GmailReadAttachmentOutput{}, err
	}
	storedAttachmentID := attachment.Attachment.ID
	if storedAttachmentID == "" {
		storedAttachmentID = attachmentID
	}
	storedPath, err := t.attachmentWriter.WriteAttachment(ctx, integrationingest.RawAttachment{
		Provider:     gmailconnector.ProviderID,
		AccountID:    session.connection.AccountID,
		ConnectionID: session.connection.ID,
		MessageID:    messageID,
		AttachmentID: storedAttachmentID,
		FileName:     attachment.Attachment.FileName,
		Data:         attachment.Data,
	})
	if err != nil {
		return nil, GmailReadAttachmentOutput{}, err
	}
	out := gmailToolAttachment(messageID, storedAttachmentID, storedPath, attachment)
	out.Connected = true
	out.Accounts = session.accounts
	out.AccountID = session.connection.AccountID
	out.Alias = session.connection.Alias

	text := "Saved Gmail attachment"
	if out.FileName != "" {
		text += ": " + out.FileName
	}
	if out.FilePath != "" {
		text += " saved to " + out.FilePath
	}
	if out.UnsupportedContent {
		text += " (content not returned in context)"
	}
	return textResult(text), out, nil
}

func gmailToolAttachment(messageID, attachmentID, filePath string, attachment gmailconnector.AttachmentContent) GmailReadAttachmentOutput {
	out := GmailReadAttachmentOutput{
		MessageID:    messageID,
		AttachmentID: attachment.Attachment.ID,
		FileName:     attachment.Attachment.FileName,
		MIMEType:     attachment.Attachment.MIMEType,
		Size:         attachment.Size,
		FilePath:     filePath,
	}
	if out.AttachmentID == "" {
		out.AttachmentID = attachmentID
	}
	if !textAttachment(out.MIMEType, out.FileName) {
		out.UnsupportedContent = true
		return out
	}
	out.TextPreview, out.TextPreviewTruncated = clampAttachmentTextPreview(cleanAttachmentText(out.MIMEType, attachment.Data))
	return out
}

func textAttachment(mimeType, fileName string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	switch {
	case strings.HasPrefix(mimeType, "text/"):
		return true
	case mimeType == "application/json", mimeType == "application/xml", mimeType == "application/csv", mimeType == "application/x-ndjson":
		return true
	case mimeType == "" && textFileName(fileName):
		return true
	default:
		return false
	}
}

func cleanAttachmentText(mimeType string, data []byte) string {
	if strings.EqualFold(strings.TrimSpace(mimeType), "text/html") {
		return emailclean.Body("", string(data))
	}
	return emailclean.Text(string(data))
}

func textFileName(fileName string) bool {
	fileName = strings.ToLower(strings.TrimSpace(fileName))
	for _, suffix := range []string{".txt", ".md", ".csv", ".json", ".xml", ".log"} {
		if strings.HasSuffix(fileName, suffix) {
			return true
		}
	}
	return false
}

func clampAttachmentTextPreview(text string) (string, bool) {
	runes := []rune(text)
	if len(runes) <= maxGmailAttachmentTextPreviewChars {
		return text, false
	}
	return string(runes[:maxGmailAttachmentTextPreviewChars]) + "...", true
}
