package mcp

import (
	"regexp"
	"strings"
)

// resourceMeta is what a UI resource's _meta declares: the origins its page
// may reach.
type resourceMeta struct {
	UI struct {
		CSP struct {
			ConnectDomains  []string `json:"connectDomains"`
			ResourceDomains []string `json:"resourceDomains"`
			FrameDomains    []string `json:"frameDomains"`
			BaseURIDomains  []string `json:"baseUriDomains"`
		} `json:"csp"`
	} `json:"ui"`
}

var (
	// Anything a server declares that is not an origin is dropped, so a
	// declaration cannot add directives of its own.
	cspOrigin = regexp.MustCompile(`^(?i:https?|wss?)://(\*|(\*\.)?[a-zA-Z0-9-]+(\.[a-zA-Z0-9-]+)*)(:(\d+|\*))?$`)
	doctype   = regexp.MustCompile(`^(?i)\s*<!doctype[^>]*>`)
)

// policy is the Content Security Policy MCP Apps requires of a host: the page
// reaches only the origins its resource declares, and none by default. The
// page runs in a sandbox with no origin of its own, so 'self' is never
// granted; data: and blob: content is the page's own.
func (m resourceMeta) policy() string {
	csp := m.UI.CSP
	return strings.Join([]string{
		"default-src 'none'",
		"script-src" + cspSources(" 'unsafe-inline'", csp.ResourceDomains),
		"style-src" + cspSources(" 'unsafe-inline'", csp.ResourceDomains),
		"img-src" + cspSources(" data: blob:", csp.ResourceDomains),
		"media-src" + cspSources(" data: blob:", csp.ResourceDomains),
		"font-src" + cspSources(" data:", csp.ResourceDomains),
		"connect-src" + cspSources("", csp.ConnectDomains),
		"frame-src" + cspSources("", csp.FrameDomains),
		"base-uri" + cspSources("", csp.BaseURIDomains),
		"form-action 'none'",
	}, "; ")
}

func cspSources(sources string, declared []string) string {
	for _, origin := range declared {
		if cspOrigin.MatchString(origin) {
			sources += " " + origin
		}
	}
	if sources == "" {
		return " 'none'"
	}
	return sources
}

// enforce puts the policy first in the document, where it governs everything
// after it.
func (m resourceMeta) enforce(html string) string {
	at := len(doctype.FindString(html))
	return html[:at] + `<meta http-equiv="Content-Security-Policy" content="` + m.policy() + `">` + html[at:]
}
