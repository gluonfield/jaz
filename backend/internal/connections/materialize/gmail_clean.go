package materialize

import (
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

const (
	maxEmailBodyChars = 12000
	maxEmailURLChars  = 160
)

var emailURLPattern = regexp.MustCompile(`https?://[^\s<>"']+`)

func cleanEmailBody(text, htmlBody string) string {
	if strings.TrimSpace(text) != "" {
		return cleanEmailText(text)
	}
	if strings.TrimSpace(htmlBody) == "" {
		return ""
	}
	return cleanEmailText(htmlEmailText(htmlBody))
}

func cleanEmailText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = emailURLPattern.ReplaceAllStringFunc(value, cleanEmailTextURL)
	lines := strings.Split(value, "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = collapseSpaces(line)
		if line == "" {
			if !blank && len(out) > 0 {
				out = append(out, "")
			}
			blank = true
			continue
		}
		out = append(out, line)
		blank = false
	}
	return truncateEmailBody(strings.TrimSpace(strings.Join(out, "\n")))
}

func htmlEmailText(raw string) string {
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
		if skippedEmailNode(name) {
			return
		}
		if name == "br" {
			b.WriteByte('\n')
			return
		}
		if name == "a" {
			writeEmailLink(&b, n)
			return
		}
		if emailBlockNode(name) {
			b.WriteByte('\n')
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if emailBlockNode(name) {
			b.WriteByte('\n')
		}
	}
	walk(node)
	return b.String()
}

func writeEmailLink(b *strings.Builder, n *html.Node) {
	text := collapseSpaces(emailNodeText(n))
	link := cleanEmailURL(emailAttr(n, "href"))
	if text == "" {
		if link != "" {
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

func emailNodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			b.WriteByte(' ')
			return
		}
		if n.Type == html.ElementNode && skippedEmailNode(strings.ToLower(n.Data)) {
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

func emailAttr(n *html.Node, name string) string {
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, name) {
			return attr.Val
		}
	}
	return ""
}

func cleanEmailURL(raw string) string {
	raw = strings.TrimSpace(raw)
	raw, _ = splitEmailURLTrailing(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return ""
	}
	if emailImageURL(parsed) {
		return ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	out := parsed.String()
	if len(out) <= maxEmailURLChars {
		return out
	}
	return shortenEmailURL(parsed)
}

func cleanEmailTextURL(raw string) string {
	base, trailing := splitEmailURLTrailing(strings.TrimSpace(raw))
	cleaned := cleanEmailURL(base)
	if cleaned == "" {
		return ""
	}
	return cleaned + trailing
}

func splitEmailURLTrailing(raw string) (string, string) {
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

func shortenEmailURL(parsed *url.URL) string {
	path := parsed.EscapedPath()
	if len(path) > 80 {
		path = path[:80] + "..."
	}
	out := parsed.Scheme + "://" + parsed.Host + path
	if len(out) <= maxEmailURLChars {
		return out
	}
	return parsed.Scheme + "://" + parsed.Host
}

func emailImageURL(parsed *url.URL) bool {
	path := strings.ToLower(parsed.Path)
	for _, suffix := range []string{".gif", ".jpg", ".jpeg", ".png", ".webp", ".svg", ".avif"} {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return strings.Contains(path, "/pixel") || strings.Contains(path, "/track") || strings.Contains(path, "/open")
}

func skippedEmailNode(name string) bool {
	switch name {
	case "head", "script", "style", "meta", "link", "img", "svg", "picture", "source", "canvas", "noscript":
		return true
	default:
		return false
	}
}

func emailBlockNode(name string) bool {
	switch name {
	case "address", "article", "aside", "blockquote", "body", "div", "footer", "h1", "h2", "h3", "h4", "h5", "h6", "header", "li", "main", "p", "section", "table", "tbody", "td", "tfoot", "th", "thead", "tr", "ul", "ol":
		return true
	default:
		return false
	}
}

func collapseSpaces(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func truncateEmailBody(value string) string {
	runes := []rune(value)
	if len(runes) <= maxEmailBodyChars {
		return value
	}
	return string(runes[:maxEmailBodyChars]) + "\n\n[truncated]"
}
