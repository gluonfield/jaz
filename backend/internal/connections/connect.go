package connections

import (
	"context"
	"fmt"

	"github.com/wins/jaz/backend/pkg/integrations"
)

type ConnectStart struct {
	Type    string          `json:"type"`
	AuthURL string          `json:"auth_url,omitempty"`
	QR      *QRStart        `json:"qr,omitempty"`
	MCP     *RemoteMCPStart `json:"mcp,omitempty"`
}

type ConnectService struct {
	catalog   *Catalog
	oauth     *OAuthService
	qr        *QRService
	remoteMCP *RemoteMCPConnector
}

func NewConnectService(catalog *Catalog, oauth *OAuthService, qr *QRService, remoteMCP *RemoteMCPConnector) *ConnectService {
	return &ConnectService{catalog: catalog, oauth: oauth, qr: qr, remoteMCP: remoteMCP}
}

func (s *ConnectService) Start(ctx context.Context, pluginID, redirectURL string) (ConnectStart, error) {
	plugin, ok := s.catalog.Plugin(pluginID)
	if !ok {
		return ConnectStart{}, fmt.Errorf("connection plugin %q is not available", pluginID)
	}
	if len(plugin.Auth) == 0 {
		return ConnectStart{}, fmt.Errorf("connection plugin %q has no sign-in method", pluginID)
	}
	switch plugin.Auth[0].Kind {
	case integrations.AuthKindOAuth:
		if plugin.Implementation.Status != "available" {
			return ConnectStart{}, fmt.Errorf("connection plugin %q is %s", pluginID, plugin.Implementation.Status)
		}
		if s.oauth == nil {
			return ConnectStart{}, fmt.Errorf("connection plugin %q does not support OAuth here", pluginID)
		}
		start, err := s.oauth.Start(ctx, pluginID, redirectURL)
		if err != nil {
			return ConnectStart{}, err
		}
		return ConnectStart{Type: "oauth", AuthURL: start.AuthURL}, nil
	case integrations.AuthKindSession:
		if s.qr == nil {
			return ConnectStart{}, fmt.Errorf("connection plugin %q does not support QR login here", pluginID)
		}
		if !s.qr.Available(pluginID) {
			return ConnectStart{}, fmt.Errorf("%w: %s", ErrQRProviderUnavailable, pluginID)
		}
		start, err := s.qr.Start(ctx, pluginID)
		if err != nil {
			return ConnectStart{}, err
		}
		return ConnectStart{Type: "qr", QR: &start}, nil
	case integrations.AuthKindRemoteMCP:
		if plugin.Implementation.Status != "available" {
			return ConnectStart{}, fmt.Errorf("connection plugin %q is %s", pluginID, plugin.Implementation.Status)
		}
		if s.remoteMCP == nil {
			return ConnectStart{}, fmt.Errorf("connection plugin %q does not support remote MCP here", pluginID)
		}
		start, err := s.remoteMCP.Connect(ctx, plugin)
		if err != nil {
			return ConnectStart{}, err
		}
		return ConnectStart{Type: "mcp", MCP: &start}, nil
	default:
		if plugin.Implementation.Status != "available" {
			return ConnectStart{}, fmt.Errorf("connection plugin %q is %s", pluginID, plugin.Implementation.Status)
		}
		return ConnectStart{}, fmt.Errorf("connection plugin %q uses unsupported sign-in method %q", pluginID, plugin.Auth[0].Kind)
	}
}
