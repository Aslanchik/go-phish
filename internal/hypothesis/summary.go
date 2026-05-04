package hypothesis

import (
	"fmt"
	"strings"

	"golang.org/x/net/html"

	"github.com/aslanchik/go-phish/internal/fetcher"
)

const visibleTextLimit = 2000

// Summary is a compact, LLM-ready extract of a rendered page.
type Summary struct {
	Title       string
	Description string
	Forms       []fetcher.Form
	VisibleText string
}

// DOMSummary extracts a structured summary from a rendered DOM and the forms
// already captured by the fetcher.
func DOMSummary(dom string, forms []fetcher.Form) (Summary, error) {
	doc, err := html.Parse(strings.NewReader(dom))
	if err != nil {
		return Summary{}, fmt.Errorf("parse DOM: %w", err)
	}

	s := Summary{
		Title:       extractTitle(doc),
		Description: extractMetaDescription(doc),
		Forms:       forms,
		VisibleText: extractVisibleText(doc),
	}
	return s, nil
}

// String serializes the summary into a compact text block for use in an LLM prompt.
func (s Summary) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Title: %s\n", s.Title)
	fmt.Fprintf(&b, "Description: %s\n", s.Description)
	if len(s.Forms) > 0 {
		b.WriteString("Forms:\n")
		for _, f := range s.Forms {
			fmt.Fprintf(&b, "  action=%s method=%s fields=[", f.Action, f.Method)
			parts := make([]string, len(f.Fields))
			for i, field := range f.Fields {
				parts[i] = field.Name + ":" + field.Type
			}
			b.WriteString(strings.Join(parts, ", "))
			b.WriteString("]\n")
		}
	}
	fmt.Fprintf(&b, "Visible text:\n%s", s.VisibleText)
	return b.String()
}

func extractTitle(doc *html.Node) string {
	var title string
	walk(doc, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "title" {
			if n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
				title = strings.TrimSpace(n.FirstChild.Data)
				return false // stop walking
			}
		}
		return true
	})
	return title
}

func extractMetaDescription(doc *html.Node) string {
	var desc string
	walk(doc, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "meta" {
			return true
		}
		var name, content string
		for _, a := range n.Attr {
			switch strings.ToLower(a.Key) {
			case "name":
				name = strings.ToLower(a.Val)
			case "content":
				content = a.Val
			}
		}
		if name == "description" {
			desc = content
			return false
		}
		return true
	})
	return desc
}

func extractVisibleText(doc *html.Node) string {
	var b strings.Builder
	walk(doc, func(n *html.Node) bool {
		// Skip invisible subtrees.
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "noscript", "head":
				return false
			}
		}
		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				if b.Len() > 0 {
					b.WriteByte(' ')
				}
				b.WriteString(text)
			}
		}
		return b.Len() < visibleTextLimit
	})
	text := b.String()
	if len(text) > visibleTextLimit {
		text = text[:visibleTextLimit]
	}
	return text
}

// walk does a depth-first traversal. The callback returns false to skip a node's children.
func walk(n *html.Node, fn func(*html.Node) bool) {
	if !fn(n) {
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, fn)
	}
}
