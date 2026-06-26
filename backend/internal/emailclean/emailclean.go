package emailclean

import (
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

const (
	MaxBodyChars = 8000
	MaxURLChars  = 160
)

var (
	urlPattern       = regexp.MustCompile(`https?://[^\s<>"']+`)
	noisyTokenURL    = regexp.MustCompile(`(?i)\b(?:cid|data):[^\s<>"']+`)
	opaqueTokenLine  = regexp.MustCompile(`^[A-Za-z0-9._~+/\-=%]{240,}$`)
	redirectParamSet = map[string]struct{}{
		"url": {}, "u": {}, "target": {}, "redirect": {}, "redirect_url": {}, "redirecturl": {}, "link": {},
	}
)

func Body(text, htmlBody string) string {
	if strings.TrimSpace(text) != "" {
		return Text(text)
	}
	if strings.TrimSpace(htmlBody) == "" {
		return ""
	}
	return Text(HTMLText(htmlBody))
}

func Text(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = noisyTokenURL.ReplaceAllString(value, "")
	value = urlPattern.ReplaceAllStringFunc(value, cleanTextURL)

	lines := strings.Split(value, "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = collapseSpaces(line)
		if line == "" || dropLine(line) {
			if !blank && len(out) > 0 {
				out = append(out, "")
			}
			blank = true
			continue
		}
		out = append(out, line)
		blank = false
	}
	return truncate(strings.TrimSpace(strings.Join(out, "\n")))
}

func HTMLText(raw string) string {
	node, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return raw
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			b.WriteByte(' ')
			return
		}
		if n.Type != html.ElementNode {
			for child := n.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
			return
		}
		name := strings.ToLower(n.Data)
		if skippedNode(name) {
			return
		}
		if name == "br" {
			b.WriteByte('\n')
			return
		}
		if name == "a" {
			writeLink(&b, n)
			return
		}
		if blockNode(name) {
			b.WriteByte('\n')
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if blockNode(name) {
			b.WriteByte('\n')
		}
	}
	walk(node)
	return b.String()
}

func writeLink(b *strings.Builder, n *html.Node) {
	text := collapseSpaces(Text(nodeText(n)))
	link := cleanURL(attr(n, "href"), 0)
	if text == "" {
		if link != "" && !imageURL(link) {
			b.WriteString(link)
			b.WriteByte(' ')
		}
		return
	}
	b.WriteString(text)
	if link != "" && !strings.EqualFold(text, link) {
		b.WriteString(" (")
		b.WriteString(link)
		b.WriteByte(')')
	}
	b.WriteByte(' ')
}

func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			b.WriteByte(' ')
			return
		}
		if n.Type == html.ElementNode && skippedNode(strings.ToLower(n.Data)) {
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		walk(child)
	}
	return b.String()
}

func attr(n *html.Node, name string) string {
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, name) {
			return attr.Val
		}
	}
	return ""
}

func cleanTextURL(raw string) string {
	base, trailing := splitTrailing(strings.TrimSpace(raw))
	cleaned := cleanURL(base, 0)
	if cleaned == "" {
		return ""
	}
	return cleaned + trailing
}

func cleanURL(raw string, depth int) string {
	raw = strings.TrimSpace(raw)
	raw, _ = splitTrailing(raw)
	if raw == "" || strings.HasPrefix(strings.ToLower(raw), "cid:") || strings.HasPrefix(strings.ToLower(raw), "data:") {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if parsed.Scheme == "mailto" {
		return cleanMailto(parsed)
	}
	if parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return ""
	}
	if depth < 2 {
		if target := redirectTarget(parsed); target != "" {
			if cleaned := cleanURL(target, depth+1); cleaned != "" {
				return cleaned
			}
		}
	}
	if trackingURL(parsed) {
		return ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	out := parsed.String()
	if len(out) <= MaxURLChars {
		return out
	}
	return shortenURL(parsed)
}

func cleanMailto(parsed *url.URL) string {
	address := strings.TrimSpace(parsed.Opaque)
	if address == "" {
		address = strings.TrimSpace(parsed.Path)
	}
	if address == "" || strings.ContainsAny(address, "\n\r") {
		return ""
	}
	return "mailto:" + address
}

func redirectTarget(parsed *url.URL) string {
	values := parsed.Query()
	for key, items := range values {
		if _, ok := redirectParamSet[strings.ToLower(strings.ReplaceAll(key, "-", "_"))]; !ok {
			continue
		}
		for _, item := range items {
			item = strings.TrimSpace(item)
			if strings.HasPrefix(item, "http://") || strings.HasPrefix(item, "https://") {
				return item
			}
		}
	}
	return ""
}

func splitTrailing(raw string) (string, string) {
	trailing := ""
	for raw != "" {
		last := raw[len(raw)-1]
		if !strings.ContainsRune(".,);]}>", rune(last)) {
			break
		}
		trailing = string(last) + trailing
		raw = raw[:len(raw)-1]
	}
	return raw, trailing
}

func shortenURL(parsed *url.URL) string {
	path := parsed.EscapedPath()
	if len(path) > 80 {
		path = path[:80] + "..."
	}
	out := parsed.Scheme + "://" + parsed.Host + path
	if len(out) <= MaxURLChars {
		return out
	}
	return parsed.Scheme + "://" + parsed.Host
}

func imageURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	path := strings.ToLower(parsed.Path)
	for _, suffix := range []string{".gif", ".jpg", ".jpeg", ".png", ".webp", ".svg", ".avif"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return false
}

func trackingURL(parsed *url.URL) bool {
	host := strings.ToLower(parsed.Hostname())
	if trackingPath(parsed.Path) {
		return true
	}
	for _, marker := range []string{"mailchimp", "sendgrid", "mandrillapp", "list-manage", "doubleclick"} {
		if strings.Contains(host, marker) {
			return true
		}
	}
	return false
}

func trackingPath(path string) bool {
	for _, segment := range strings.Split(strings.ToLower(path), "/") {
		switch {
		case segment == "pixel", segment == "track", segment == "tracking", segment == "open", segment == "beacon":
			return true
		case strings.HasPrefix(segment, "pixel."), strings.HasPrefix(segment, "track."), strings.HasPrefix(segment, "tracking."), strings.HasPrefix(segment, "open."), strings.HasPrefix(segment, "beacon."):
			return true
		}
	}
	return false
}

func skippedNode(name string) bool {
	switch name {
	case "head", "script", "style", "meta", "link", "img", "svg", "picture", "source", "canvas", "noscript", "iframe":
		return true
	default:
		return false
	}
}

func blockNode(name string) bool {
	switch name {
	case "address", "article", "aside", "blockquote", "body", "div", "footer", "h1", "h2", "h3", "h4", "h5", "h6", "header", "li", "main", "p", "section", "table", "tbody", "td", "tfoot", "th", "thead", "tr", "ul", "ol":
		return true
	default:
		return false
	}
}

func dropLine(line string) bool {
	lower := strings.ToLower(line)
	if opaqueTokenLine.MatchString(line) {
		return true
	}
	if len(line) <= 220 {
		for _, phrase := range []string{
			"unsubscribe", "manage preferences", "email preferences", "view this email in your browser",
			"privacy policy", "terms of service", "why am i receiving this", "sent via mailchimp",
		} {
			if strings.Contains(lower, phrase) {
				return true
			}
		}
	}
	return false
}

func collapseSpaces(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func truncate(value string) string {
	runes := []rune(value)
	if len(runes) <= MaxBodyChars {
		return value
	}
	return string(runes[:MaxBodyChars]) + "\n\n[truncated]"
}
