package connections

import (
	"context"
	"errors"
	"fmt"

	"github.com/wins/jaz/backend/internal/mcpconfig"
	"github.com/wins/jaz/backend/pkg/integrations"
)

type ConnectStart struct {
	Type    string   `json:"type"`
	AuthURL string   `json:"auth_url,omitempty"`
	QR      *QRStart `json:"qr,omitempty"`
}

type ConnectResult struct {
	Start             ConnectStart
	MCPServersChanged bool
}

type StartOptions struct {
	OAuthRedirectURL string
	MCP              mcpconfig.AuthorizeOptions
}

type ConnectService struct {
	catalog       *Catalog
	oauth         *OAuthService
	qr            *QRService
	mcpConnection *MCPConnectionConnector
}

func NewConnectService(catalog *Catalog, oauth *OAuthService, qr *QRService, mcpConnection *MCPConnectionConnector) *ConnectService {
	return &ConnectService{catalog: catalog, oauth: oauth, qr: qr, mcpConnection: mcpConnection}
}

func (s *ConnectService) Start(ctx context.Context, pluginID string, opts StartOptions) (ConnectResult, error) {
	plugin, ok := s.catalog.Plugin(pluginID)
	if !ok {
		return ConnectResult{}, fmt.Errorf("plugin %q is not a native integration", pluginID)
	}
	authKind := plugin.PrimaryAuthKind()
	if authKind == "" {
		return ConnectResult{}, fmt.Errorf("plugin %q has no sign-in method", pluginID)
	}
	if !plugin.Available() && authKind != integrations.AuthKindSession {
		if plugin.Implementation.Reason != "" {
			return ConnectResult{}, errors.New(plugin.Implementation.Reason)
		}
		return ConnectResult{}, fmt.Errorf("plugin %q is %s", pluginID, plugin.Implementation.Status)
	}
	switch authKind {
	case integrations.AuthKindOAuth:
		if s.oauth == nil {
			return ConnectResult{}, fmt.Errorf("plugin %q does not support OAuth here", pluginID)
		}
		start, err := s.oauth.Start(ctx, pluginID, opts.OAuthRedirectURL)
		if err != nil {
			return ConnectResult{}, err
		}
		return ConnectResult{Start: ConnectStart{Type: "oauth", AuthURL: start.AuthURL}}, nil
	case integrations.AuthKindSession:
		if s.qr == nil {
			return ConnectResult{}, fmt.Errorf("plugin %q does not support QR login here", pluginID)
		}
		if !s.qr.Available(pluginID) {
			return ConnectResult{}, fmt.Errorf("%w: %s", ErrQRProviderUnavailable, pluginID)
		}
		start, err := s.qr.Start(ctx, pluginID)
		if err != nil {
			return ConnectResult{}, err
		}
		return ConnectResult{Start: ConnectStart{Type: "qr", QR: &start}}, nil
	default:
		return ConnectResult{}, fmt.Errorf("plugin %q uses unsupported sign-in method %q", pluginID, authKind)
	}
}

func (s *ConnectService) StartMCP(ctx context.Context, pluginID, serverID, name, rawURL string, opts mcpconfig.AuthorizeOptions) (ConnectResult, error) {
	if s.mcpConnection == nil {
		return ConnectResult{}, fmt.Errorf("plugin %q does not support MCP sign-in here", pluginID)
	}
	client := s.catalog.OAuthClientByID(pluginID)
	start, err := s.mcpConnection.ConnectServer(ctx, pluginID, serverID, name, rawURL, client, opts)
	if err != nil {
		return ConnectResult{}, err
	}
	return ConnectResult{Start: ConnectStart{Type: "oauth", AuthURL: start.AuthURL}}, nil
}
