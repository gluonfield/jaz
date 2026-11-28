package app

import (
	"context"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/browsercontrol"
	"github.com/wins/jaz/backend/internal/cloudsync"
	vaultapi "github.com/wins/jaz/backend/internal/httpapi/vault"
	"github.com/wins/jaz/backend/internal/jaztools"
	vaultcore "github.com/wins/jaz/backend/internal/vault"
	"go.uber.org/fx"
)

type vaultHandlerDeps struct {
	fx.In

	Service *vaultcore.Service
	Cloud   *cloudsync.Service `optional:"true"`
}

func NewVaultHandler(deps vaultHandlerDeps) *vaultapi.Handler {
	h := vaultapi.NewHandler(deps.Service)
	if deps.Cloud != nil {
		h.WithCloud(deps.Cloud)
	}
	return h
}

type browserVaultFiller struct {
	backend *browsercontrol.ConfiguredBackend
}

func (f browserVaultFiller) ActivePageOrigin(ctx context.Context, sessionID string) (string, error) {
	return f.backend.ActivePageOrigin(ctx, sessionID)
}

func (f browserVaultFiller) BlindFill(ctx context.Context, sessionID string, req vaultcore.BlindFillRequest) (vaultcore.BlindFillResult, error) {
	fields := make([]browsercontrol.VaultFillField, len(req.Fields))
	for i, field := range req.Fields {
		fields[i] = browsercontrol.VaultFillField{
			Slot:  field.Slot,
			Ref:   field.Ref,
			Value: field.Value,
		}
	}
	defer func() {
		for i := range fields {
			fields[i].Value = ""
		}
	}()
	res, err := f.backend.BlindFill(ctx, sessionID, browsercontrol.VaultFillRequest{
		ExpectedOrigin:      req.ExpectedOrigin,
		AllowPaymentIframes: req.AllowPaymentIframes,
		Fields:              fields,
	})
	if err != nil {
		return vaultcore.BlindFillResult{}, err
	}
	return vaultcore.BlindFillResult{FilledSlots: res.FilledSlots}, nil
}

func ConfigureVaultTools(jaz *jaztools.Service, service *vaultcore.Service, backend *browsercontrol.ConfiguredBackend, manager *acp.Manager) {
	var filler vaultcore.BrowserFiller
	if backend != nil {
		filler = browserVaultFiller{backend: backend}
	}
	jaz.SetVault(service, filler, manager)
}
