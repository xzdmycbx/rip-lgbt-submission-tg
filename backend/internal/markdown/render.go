// Package markdown ports the custom memorial markdown engine from the
// original Cloudflare Workers frontend.js. It accepts memorial body markdown
// (with frontmatter pre-stripped) and a personPath used to resolve relative
// asset URLs, returning HTML that the Vue layer slots in via v-html.
package markdown

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Render parses cleaned memorial markdown (frontmatter already stripped) and
// returns rendered HTML. Mirrors frontend.js renderMarkdown.
func Render(md string, personPath string) string {
	pre := preprocess(md)
	lines := strings.Split(pre, "\n")
	body, footnotes := extractFootnotes(lines)

	html := renderLines(body, personPath)
	if len(footnotes) > 0 {
		var b strings.Builder
		b.WriteString(`<section class="footnotes" aria-label="脚注"><ol>`)
		for _, n := range footnotes {
			fmt.Fprintf(&b, `<li id="fn-%s">%s</li>`, escapeAttr(n.ID), renderInline(n.Content, personPath))
		}
		b.WriteString(`</ol></section>`)
		html += b.String()
	}
	return strings.TrimSpace(html)
}

// StripFrontmatter removes a leading YAML frontmatter block delimited by ---.
func StripFrontmatter(md string) string {
	text := strings.ReplaceAll(md, "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return strings.TrimSpace(text)
	}
	end := strings.Index(text[4:], "\n---")
	if end == -1 {
		return strings.TrimSpace(text)
	}
	end += 4
	rest := text[end+4:]
	if i := strings.Index(rest, "\n"); i >= 0 {
		return strings.TrimSpace(rest[i+1:])
	}
	return ""
}

// CleanMemorial trims and normalizes line endings.
func CleanMemorial(md string) string {
	return strings.TrimSpace(strings.ReplaceAll(md, "\r\n", "\n"))
}

type footnote struct {
	ID      string
	Content string
}

func extractFootnotes(lines []string) ([]string, []footnote) {
	body := make([]string, 0, len(lines))
	var notes []footnote

	re := regexp.MustCompile(`^\[\^([^\]]+)\]:\s*(.*)$`)
	indent := regexp.MustCompile(`^\s+`)

	for i := 0; i < len(lines); i++ {
		m := re.FindStringSubmatch(lines[i])
		if m == nil {
			body = append(body, lines[i])
			continue
		}
		id := m[1]
		parts := []string{m[2]}
		for i+1 < len(lines) && indent.MatchString(lines[i+1]) && strings.TrimSpace(lines[i+1]) != "" {
			parts = append(parts, strings.TrimSpace(lines[i+1]))
			i++
		}
		notes = append(notes, footnote{ID: id, Content: strings.Join(parts, " ")})
	}
	return body, notes
}

// encodeTokenData serializes data exactly the way frontend.js does:
// encodeURIComponent(JSON.stringify(data)).replace(/\*/g, '%2A').
func encodeTokenData(v any) string {
	raw, _ := json.Marshal(v)
	return strings.ReplaceAll(url.QueryEscape(string(raw)), "+", "%20")
}

func decodeTokenData(value string) map[string]any {
	out := map[string]any{}
	dec, err := url.QueryUnescape(value)
	if err != nil {
		return out
	}
	_ = json.Unmarshal([]byte(dec), &out)
	return out
}

func tokenString(data map[string]any, key string) string {
	if v, ok := data[key]; ok {
		switch t := v.(type) {
		case string:
			return t
		case float64:
			return fmt.Sprintf("%g", t)
		case bool:
			if t {
				return "true"
			}
			return "false"
		}
	}
	return ""
}

func tokenStringSlice(data map[string]any, key string) []string {
	v, ok := data[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(v))
	for _, item := range v {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func tokenBool(data map[string]any, key string) bool {
	if v, ok := data[key].(bool); ok {
		return v
	}
	return false
}
