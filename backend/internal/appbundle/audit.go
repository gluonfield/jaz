package appbundle

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"
)

var (
	externalURLRe = regexp.MustCompile(`https?://[^\s"'` + "`" + `<>)\]}]+`)
	localPathRe   = regexp.MustCompile(`(?i)(/Users/[A-Za-z0-9._-]+/|/home/[A-Za-z0-9._-]+/|[A-Z]:\\[A-Za-z0-9._\\-]+|file://)`)
)

func isAuditableTextFile(relpath string) bool {
	switch strings.ToLower(path.Ext(relpath)) {
	case ".html", ".htm", ".js", ".mjs", ".css", ".svg", ".md", ".markdown", ".json", ".txt":
		return true
	default:
		return false
	}
}

func IsAllowlistedCDNHost(host string) bool {
	return allowlistedCDNHosts[strings.ToLower(strings.TrimSpace(host))]
}

func AuditFiles(files []FileEntry, manifest Manifest) ([]ExternalURL, []string) {
	urlSet := make(map[string]ExternalURL)
	warnSet := make(map[string]bool)

	for _, f := range files {
		if f.Path == ManifestPath || !isAuditableTextFile(f.Path) {
			continue
		}
		text := string(f.Data)
		lower := strings.ToLower(text)

		for _, raw := range externalURLRe.FindAllString(text, -1) {
			cleaned := strings.TrimRight(raw, ".,;:")
			parsed, err := url.Parse(cleaned)
			if err != nil || parsed.Host == "" {
				continue
			}
			host := strings.ToLower(parsed.Hostname())
			allowlisted := IsAllowlistedCDNHost(host)
			if _, exists := urlSet[cleaned]; !exists {
				urlSet[cleaned] = ExternalURL{
					URL:         cleaned,
					Host:        host,
					Allowlisted: allowlisted,
					SourceFile:  f.Path,
				}
			}
			if !allowlisted {
				warnSet[fmt.Sprintf("external non-CDN host referenced in %s: %s (blocked by default CSP)", f.Path, host)] = true
			}
		}

		if localPathRe.MatchString(text) {
			warnSet[fmt.Sprintf("possible local filesystem path detected in %s", f.Path)] = true
		}
		if strings.Contains(lower, "navigator.serviceworker") {
			warnSet[fmt.Sprintf("serviceWorker API referenced in %s (blocked by sandbox host)", f.Path)] = true
		}
		if strings.Contains(lower, "document.cookie") {
			warnSet[fmt.Sprintf("document.cookie referenced in %s (content origin is cookieless)", f.Path)] = true
		}
		if strings.Contains(lower, "http-equiv=\"content-security-policy\"") || strings.Contains(lower, "http-equiv='content-security-policy'") {
			warnSet[fmt.Sprintf("meta CSP tag found in %s (host HTTP header CSP cannot be loosened)", f.Path)] = true
		}
	}

	for _, capName := range manifest.Capabilities {
		switch capName {
		case CapStorageShared:
			warnSet["app declares shared storage: writes are visible to all viewers with access to this app"] = true
		case CapAIComplete:
			warnSet["app declares AI completions: calls are billed against each viewer's org/account quota"] = true
		case CapToolsCall:
			warnSet["app declares tool calls: viewer's own connector auth will be used for allowlisted tools"] = true
		case CapDesktopFullAccess:
			warnSet["app declares desktop:full_access: requires org admin policy and per-version user re-approval"] = true
		}
	}

	urls := make([]ExternalURL, 0, len(urlSet))
	for _, u := range urlSet {
		urls = append(urls, u)
	}
	sort.Slice(urls, func(i, j int) bool {
		if urls[i].URL != urls[j].URL {
			return urls[i].URL < urls[j].URL
		}
		return urls[i].SourceFile < urls[j].SourceFile
	})

	warnings := make([]string, 0, len(warnSet))
	for w := range warnSet {
		warnings = append(warnings, w)
	}
	sort.Strings(warnings)

	return urls, warnings
}
