package markdown

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// renderInline mirrors renderInline() in frontend.js. It produces HTML for
// content within a single block, handling images, links, code, bold/em,
// footnote references, and the inline data tokens emitted during preprocess.
func renderInline(value, personPath string) string {
	source := value

	// <ruby>...<rt>...</rt></ruby> -> [[RUBY:...]]
	source = regexp.MustCompile(`(?is)<ruby>([\s\S]*?)<rt>([\s\S]*?)</rt></ruby>`).ReplaceAllStringFunc(source, func(m string) string {
		sub := regexp.MustCompile(`(?is)<ruby>([\s\S]*?)<rt>([\s\S]*?)</rt></ruby>`).FindStringSubmatch(m)
		if len(sub) < 3 {
			return ""
		}
		return dataInlineToken("RUBY", map[string]any{
			"base": cleanHTMLFragment(sub[1]),
			"rt":   cleanHTMLFragment(sub[2]),
		})
	})

	// <span style="...">...</span> -> [[SPAN_STYLE:...]]
	source = regexp.MustCompile(`(?is)<span\b([^>]*)>([\s\S]*?)</span>`).ReplaceAllStringFunc(source, func(m string) string {
		sub := regexp.MustCompile(`(?is)<span\b([^>]*)>([\s\S]*?)</span>`).FindStringSubmatch(m)
		if len(sub) < 3 {
			return ""
		}
		return dataInlineToken("SPAN_STYLE", map[string]any{
			"style":   sanitizeInlineStyle(readHTMLAttr(sub[1], "style")),
			"content": cleanHTMLFragment(sub[2]),
		})
	})

	// <br> -> \n
	source = regexp.MustCompile(`(?i)<br\s*/?>`).ReplaceAllString(source, "\n")

	html := escapeHTML(source)

	// images
	html = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`).ReplaceAllStringFunc(html, func(m string) string {
		sub := regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`).FindStringSubmatch(m)
		if len(sub) < 3 {
			return ""
		}
		src := toContentAssetURL(decodeHTMLEntities(sub[2]), personPath)
		if src == "" {
			return ""
		}
		return fmt.Sprintf(`<img class="story-inline-image" src="%s" alt="%s" loading="lazy" decoding="async">`,
			escapeAttr(src), escapeAttr(decodeHTMLEntities(sub[1])))
	})

	// links
	html = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`).ReplaceAllStringFunc(html, func(m string) string {
		sub := regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`).FindStringSubmatch(m)
		if len(sub) < 3 {
			return ""
		}
		href := decodeHTMLEntities(sub[2])
		if isSafeURL(href) {
			return fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener">%s</a>`, escapeAttr(href), sub[1])
		}
		return sub[1]
	})

	html = regexp.MustCompile("`([^`]+)`").ReplaceAllString(html, "<code>$1</code>")
	html = regexp.MustCompile(`\*\*([^*]+)\*\*`).ReplaceAllString(html, "<strong>$1</strong>")
	html = regexp.MustCompile(`(^|[^*])\*([^*\n]+)\*`).ReplaceAllString(html, "$1<em>$2</em>")
	html = regexp.MustCompile(`\[\^([^\]]+)\]`).ReplaceAllString(html, "<sup>[$1]</sup>")

	html = renderInlineDataTokens(html, personPath)
	return html
}

func renderInlineDataTokens(htmlIn, personPath string) string {
	htmlIn = regexp.MustCompile(`\[\[HTML_P:([^\]]+)\]\]`).ReplaceAllStringFunc(htmlIn, func(m string) string {
		sub := regexp.MustCompile(`\[\[HTML_P:([^\]]+)\]\]`).FindStringSubmatch(m)
		if len(sub) < 2 {
			return ""
		}
		data := decodeTokenData(sub[1])
		classes := []string{"story-inline-html-paragraph"}
		if a := tokenString(data, "align"); a != "" {
			classes = append(classes, "story-align-"+a)
		}
		if tokenBool(data, "inline") {
			classes = append(classes, "story-inline-paragraph")
		}
		return fmt.Sprintf(`<span class="%s">%s</span>`, escapeAttr(strings.Join(classes, " ")),
			strings.ReplaceAll(renderInline(tokenString(data, "content"), personPath), "\n", "<br>"))
	})
	htmlIn = regexp.MustCompile(`\[\[RUBY:([^\]]+)\]\]`).ReplaceAllStringFunc(htmlIn, func(m string) string {
		sub := regexp.MustCompile(`\[\[RUBY:([^\]]+)\]\]`).FindStringSubmatch(m)
		if len(sub) < 2 {
			return ""
		}
		data := decodeTokenData(sub[1])
		return fmt.Sprintf(`<ruby>%s<rt>%s</rt></ruby>`,
			renderInline(tokenString(data, "base"), personPath),
			escapeHTML(tokenString(data, "rt")))
	})
	htmlIn = regexp.MustCompile(`\[\[SPAN_STYLE:([^\]]+)\]\]`).ReplaceAllStringFunc(htmlIn, func(m string) string {
		sub := regexp.MustCompile(`\[\[SPAN_STYLE:([^\]]+)\]\]`).FindStringSubmatch(m)
		if len(sub) < 2 {
			return ""
		}
		data := decodeTokenData(sub[1])
		style := tokenString(data, "style")
		styleAttr := ""
		if style != "" {
			styleAttr = fmt.Sprintf(` style="%s"`, escapeAttr(style))
		}
		return fmt.Sprintf(`<span class="story-inline-style"%s>%s</span>`, styleAttr,
			renderInline(tokenString(data, "content"), personPath))
	})
	return htmlIn
}

// sanitizeInlineStyle whitelists the same handful of declarations frontend.js does.
func sanitizeInlineStyle(value string) string {
	style := strings.ToLower(value)
	var out []string

	if m := regexp.MustCompile(`background\s*:\s*linear-gradient\(([^;]+)\)`).FindStringSubmatch(style); m != nil {
		bg := strings.TrimSpace(m[1])
		if regexp.MustCompile(`^[-#%,.\s\w()]+$`).MatchString(bg) {
			out = append(out, "--story-gradient: linear-gradient("+bg+")")
		}
	}
	if regexp.MustCompile(`font-weight\s*:\s*bold`).MatchString(style) {
		out = append(out, "font-weight: 800")
	}
	if regexp.MustCompile(`background-clip\s*:\s*text`).MatchString(style) {
		out = append(out, "background: var(--story-gradient)")
		out = append(out, "-webkit-background-clip: text")
		out = append(out, "background-clip: text")
	}
	if regexp.MustCompile(`color\s*:\s*transparent`).MatchString(style) {
		out = append(out, "color: transparent")
	}
	return strings.Join(out, "; ")
}

func decodeHTMLEntities(value string) string {
	r := strings.NewReplacer(
		"&amp;", "&",
		"&quot;", `"`,
		"&#39;", "'",
		"&lt;", "<",
		"&gt;", ">",
	)
	return r.Replace(value)
}

// keep url import alive when renderLines imports url and we share helpers.
var _ = url.QueryEscape
