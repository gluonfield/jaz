package acp

import (
	"context"
	"fmt"
	"strings"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

const (
	vaultApproveOptionID = "approve_once"
	vaultDenyOptionID    = "deny"
	vaultSavedPrefix     = "saved:"
)

func (m *Manager) ConfirmVaultFill(ctx context.Context, sessionID string, approval sessionevents.VaultApprovalSpec) (bool, error) {
	if sessionID == "" {
		return false, fmt.Errorf("vault approval requires a current Jaz thread")
	}
	job := m.jobByID(sessionID)
	if job == nil {
		return false, fmt.Errorf("current Jaz thread is not active")
	}
	spec := approval
	title := "Approve payment card autofill"
	if spec.Label != "" {
		title = "Approve " + spec.Label
	}
	answer := m.awaitPermissionAnswer(ctx, job, sessionevents.ACPPermission{
		ID:            newPermissionID(),
		Title:         title,
		Status:        "pending",
		VaultApproval: &spec,
		Options: []sessionevents.ACPPermissionOption{
			{ID: vaultApproveOptionID, Name: "Approve once", Kind: "allow_once"},
			{ID: vaultDenyOptionID, Name: "Deny", Kind: "reject_once"},
		},
	}, nil)
	return answer.OptionID == vaultApproveOptionID, nil
}

func (m *Manager) RequestVaultItem(ctx context.Context, sessionID string, req sessionevents.VaultRequestSpec) (string, bool, error) {
	if sessionID == "" {
		return "", false, fmt.Errorf("vault item request requires a current Jaz thread")
	}
	job := m.jobByID(sessionID)
	if job == nil {
		return "", false, fmt.Errorf("current Jaz thread is not active")
	}
	spec := req
	title := "Save to personal Vault"
	if spec.Label != "" {
		title = "Save " + spec.Label + " to Vault"
	}
	answer := m.awaitPermissionAnswer(ctx, job, sessionevents.ACPPermission{
		ID:           newPermissionID(),
		Title:        title,
		Status:       "pending",
		VaultRequest: &spec,
		Options: []sessionevents.ACPPermissionOption{
			{ID: vaultDenyOptionID, Name: "Cancel", Kind: "reject_once"},
		},
	}, nil)
	if strings.HasPrefix(answer.OptionID, vaultSavedPrefix) {
		itemID := strings.TrimSpace(strings.TrimPrefix(answer.OptionID, vaultSavedPrefix))
		if itemID != "" {
			return itemID, false, nil
		}
	}
	return "", true, nil
}

func isVaultSavedOptionID(optionID string) bool {
	if !strings.HasPrefix(optionID, vaultSavedPrefix) {
		return false
	}
	itemID := strings.TrimPrefix(optionID, vaultSavedPrefix)
	return itemID != "" && !strings.ContainsAny(itemID, " \t\r\n")
}
