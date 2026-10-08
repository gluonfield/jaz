package bots

import (
	"bufio"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	htmlrenderer "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

var namedMention = regexp.MustCompile(`\[@([^\[\]\r\n]+)\]`)

// linkMentions points each [@Name] that names exactly one member at that
// member's bot, so the mention keeps its recipient when bots are renamed.
func (s *Service) linkMentions(message string, members []string) string {
	names := make(map[string]string, len(members))
	for _, id := range members {
		name := s.name(id)
		if _, exists := names[name]; exists {
			names[name] = ""
		} else {
			names[name] = id
		}
	}
	source := []byte(message)
	var linked strings.Builder
	written := 0
	tree := goldmark.DefaultParser().Parse(text.NewReader(source))
	_ = ast.Walk(tree, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := node.(type) {
		case *ast.Link, *ast.CodeSpan, *ast.Image:
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			if previous, ok := node.PreviousSibling().(*ast.Text); ok && previous.Segment.Stop == node.Segment.Start {
				break
			}
			end := node.Segment.Stop
			for next := node.NextSibling(); next != nil; next = next.NextSibling() {
				run, ok := next.(*ast.Text)
				if !ok || run.Segment.Start != end {
					break
				}
				end = run.Segment.Stop
			}
			start := node.Segment.Start
			for _, match := range namedMention.FindAllSubmatchIndex(source[start:end], -1) {
				if id := names[mentionName(source[start+match[2]:start+match[3]])]; id != "" {
					stop := start + match[1]
					linked.Write(source[written:stop])
					fmt.Fprintf(&linked, "(bot:%s)", id)
					written = stop
				}
			}
		}
		return ast.WalkContinue, nil
	})
	if written == 0 {
		return message
	}
	linked.Write(source[written:])
	return linked.String()
}

func mentionName(raw []byte) string {
	var escaped strings.Builder
	writer := bufio.NewWriter(&escaped)
	htmlrenderer.DefaultWriter.Write(writer, raw)
	_ = writer.Flush()
	return html.UnescapeString(escaped.String())
}
