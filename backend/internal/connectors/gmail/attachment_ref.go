package gmail

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/wins/jaz/backend/pkg/integrations"
)

const AttachmentSourceRefPrefix = "att:gmail/"

type AttachmentSourceRef struct {
	Account   string
	MessageID string
	Index     int
}

func FormatAttachmentSourceRef(accountID, messageID string, index int) string {
	account := integrations.NormalizeAlias(accountID)
	if account == "" || messageID == "" || index <= 0 {
		return ""
	}
	return fmt.Sprintf("%s%s/%s/%d", AttachmentSourceRefPrefix, account, url.PathEscape(messageID), index)
}

func ParseAttachmentSourceRef(value string) (AttachmentSourceRef, bool, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, AttachmentSourceRefPrefix) {
		return AttachmentSourceRef{}, false, nil
	}
	parts := strings.Split(strings.TrimPrefix(value, AttachmentSourceRefPrefix), "/")
	if len(parts) != 3 {
		return AttachmentSourceRef{}, true, fmt.Errorf("invalid Gmail attachment ref")
	}
	index, err := strconv.Atoi(parts[2])
	if err != nil || index <= 0 {
		return AttachmentSourceRef{}, true, fmt.Errorf("invalid Gmail attachment ref index")
	}
	if parts[0] == "" || parts[1] == "" {
		return AttachmentSourceRef{}, true, fmt.Errorf("invalid Gmail attachment ref")
	}
	messageID, err := url.PathUnescape(parts[1])
	if err != nil || messageID == "" {
		return AttachmentSourceRef{}, true, fmt.Errorf("invalid Gmail attachment ref")
	}
	return AttachmentSourceRef{Account: parts[0], MessageID: messageID, Index: index}, true, nil
}
