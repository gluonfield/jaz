package preview

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/wins/jaz/backend/internal/httpapi"
)

const (
	capabilityPlaceholder = "{id}"
	localHostPrefix       = "jaz-preview-"
)

type originTemplate struct {
	scheme       string
	hostTemplate string
}

func parseOriginTemplate(raw string) (*originTemplate, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if strings.Count(raw, capabilityPlaceholder) != 1 {
		return nil, fmt.Errorf("preview URL template must contain {id} exactly once")
	}
	const sentinel = "jazpreviewcapabilityid"
	parsed, err := url.Parse(strings.Replace(raw, capabilityPlaceholder, sentinel, 1))
	if err != nil {
		return nil, fmt.Errorf("invalid preview URL template: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("preview URL template must use http or https")
	}
	if parsed.User != nil || parsed.Host == "" || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("preview URL template must be an origin with no credentials, path, query, or fragment")
	}
	hostname := strings.ToLower(parsed.Hostname())
	if !strings.Contains(hostname, sentinel) {
		return nil, fmt.Errorf("preview URL template must put {id} in the hostname")
	}
	if !validHostname(strings.Replace(hostname, sentinel, strings.Repeat("a", 32), 1)) {
		return nil, fmt.Errorf("preview URL template hostname is invalid")
	}
	hostTemplate := strings.Replace(parsed.Hostname(), sentinel, capabilityPlaceholder, 1)
	if port := parsed.Port(); port != "" {
		hostTemplate = net.JoinHostPort(hostTemplate, port)
	}
	return &originTemplate{
		scheme:       parsed.Scheme,
		hostTemplate: hostTemplate,
	}, nil
}

func (t *originTemplate) previewURL(id string, target *url.URL) string {
	return previewURL(t.scheme, strings.Replace(t.hostTemplate, capabilityPlaceholder, id, 1), target)
}

func localPreviewURL(r *http.Request, id string, target *url.URL) (string, bool) {
	base, err := url.Parse(httpapi.RequestBaseURL(r))
	if err != nil || !loopbackHost(base.Hostname()) {
		return "", false
	}
	host := localHostPrefix + id + ".localhost"
	if port := base.Port(); port != "" {
		host = net.JoinHostPort(host, port)
	}
	return previewURL(base.Scheme, host, target), true
}

func previewURL(scheme, host string, target *url.URL) string {
	result := &url.URL{
		Scheme:   scheme,
		Host:     host,
		Path:     target.Path,
		RawPath:  target.RawPath,
		RawQuery: target.RawQuery,
		Fragment: target.Fragment,
	}
	return result.String()
}

func previewHost(source string) (string, error) {
	parsed, err := url.Parse(source)
	if err != nil {
		return "", fmt.Errorf("invalid generated preview URL: %w", err)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("generated preview URL has no host")
	}
	return canonicalHost(parsed.Host), nil
}

func canonicalHost(host string) string {
	return strings.ToLower(strings.TrimSpace(host))
}

func validHostname(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
				return false
			}
		}
	}
	return true
}
