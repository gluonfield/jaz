package slack

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const (
	ProviderID   = "slack"
	ProviderName = "Slack"

	// OAuthClientID is Jaz's internal Slack app. Slack's MCP server rejects
	// dynamic client registration, so the client ID is pinned; it is a public
	// client used with authorization-code + PKCE and carries no secret.
	OAuthClientID = "11223932874336.11401908920209"

	// v2_user endpoints mint a user token (xoxp) for a public PKCE client.
	OAuthAuthURL  = "https://slack.com/oauth/v2_user/authorize"
	OAuthTokenURL = "https://slack.com/api/oauth.v2.user.access"
	authTestURL   = "https://slack.com/api/auth.test"

	// RemoteMCPURL is Slack's official MCP server; the stored user token
	// authenticates requests to it.
	RemoteMCPURL = "https://mcp.slack.com/mcp"
)

// UserScopes are the user-token scopes Slack's MCP tools require.
var UserScopes = []string{
	"channels:read",
	"channels:history",
	"groups:read",
	"groups:history",
	"im:read",
	"im:history",
	"mpim:read",
	"mpim:history",
	"users:read",
	"users:read.email",
	"search:read.public",
	"search:read.private",
	"search:read.im",
	"search:read.mpim",
	"chat:write",
}

type OAuthClientConfig struct {
	ClientID string
}

// Resolve returns the configured client ID, or the bundled Jaz client ID.
func (c OAuthClientConfig) Resolve() string {
	if id := strings.TrimSpace(c.ClientID); id != "" {
		return id
	}
	return OAuthClientID
}

// AuthorizeURL builds the consent URL for the public PKCE client.
func AuthorizeURL(clientID, redirectURL, state, verifier string) string {
	q := url.Values{
		"client_id":             {clientID},
		"redirect_uri":          {redirectURL},
		"state":                 {state},
		"scope":                 {strings.Join(UserScopes, ",")},
		"response_type":         {"code"},
		"code_challenge":        {pkceChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}
	return OAuthAuthURL + "?" + q.Encode()
}

// Exchange trades the authorization code for a Slack user token. Slack wraps
// the response in an {ok, ...} envelope rather than a bare OAuth2 token, so the
// exchange is done directly instead of through golang.org/x/oauth2.
func Exchange(ctx context.Context, httpClient *http.Client, clientID, redirectURL, code, verifier string) (string, error) {
	return exchangeAt(ctx, httpClient, OAuthTokenURL, clientID, redirectURL, code, verifier)
}

func exchangeAt(ctx context.Context, httpClient *http.Client, endpoint, clientID, redirectURL, code, verifier string) (string, error) {
	form := url.Values{
		"client_id":     {clientID},
		"code":          {code},
		"redirect_uri":  {redirectURL},
		"code_verifier": {verifier},
	}
	var body struct {
		OK          bool   `json:"ok"`
		Error       string `json:"error"`
		AccessToken string `json:"access_token"`
		AuthedUser  struct {
			AccessToken string `json:"access_token"`
		} `json:"authed_user"`
	}
	if err := postForm(ctx, httpClient, endpoint, "", form, &body); err != nil {
		return "", err
	}
	if !body.OK {
		return "", fmt.Errorf("slack token exchange failed: %s", slackError(body.Error))
	}
	accessToken := body.AccessToken
	if accessToken == "" {
		accessToken = body.AuthedUser.AccessToken
	}
	if accessToken == "" {
		return "", errors.New("slack token exchange returned no access token")
	}
	return accessToken, nil
}

type Profile struct {
	TeamID   string
	TeamName string
	UserID   string
	UserName string
}

func (p Profile) AccountID() string {
	return strings.TrimSpace(p.TeamID + "-" + p.UserID)
}

func (p Profile) AccountName() string {
	team := strings.TrimSpace(p.TeamName)
	user := strings.TrimSpace(p.UserName)
	switch {
	case team != "" && user != "":
		return team + " / " + user
	case team != "":
		return team
	default:
		return user
	}
}

// Identify verifies the token and resolves the workspace/user via auth.test.
func Identify(ctx context.Context, httpClient *http.Client, accessToken string) (Profile, error) {
	return identifyAt(ctx, httpClient, authTestURL, accessToken)
}

func identifyAt(ctx context.Context, httpClient *http.Client, endpoint, accessToken string) (Profile, error) {
	var body struct {
		OK     bool   `json:"ok"`
		Error  string `json:"error"`
		Team   string `json:"team"`
		TeamID string `json:"team_id"`
		User   string `json:"user"`
		UserID string `json:"user_id"`
	}
	if err := postForm(ctx, httpClient, endpoint, accessToken, url.Values{}, &body); err != nil {
		return Profile{}, err
	}
	if !body.OK {
		return Profile{}, fmt.Errorf("slack auth.test failed: %s", slackError(body.Error))
	}
	if strings.TrimSpace(body.UserID) == "" {
		return Profile{}, errors.New("slack auth.test returned no user")
	}
	return Profile{TeamID: body.TeamID, TeamName: body.Team, UserID: body.UserID, UserName: body.User}, nil
}

func postForm(ctx context.Context, httpClient *http.Client, endpoint, bearer string, form url.Values, out any) error {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode slack response: %w", err)
	}
	return nil
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func slackError(code string) string {
	if strings.TrimSpace(code) == "" {
		return "unknown error"
	}
	return code
}
