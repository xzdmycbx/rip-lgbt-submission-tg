package markdown

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// preprocess mirrors preprocessMarkdown() in frontend.js: it converts custom
// HTML/JSX-ish tags into [[NAME:payload]] block tokens and strips others.
func preprocess(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")

	text = reHTMLComment.ReplaceAllString(text, "\n")

	text = reHiddenTag.ReplaceAllStringFunc(text, func(m string) string {
		sub := reHiddenTag.FindStringSubmatch(m)
		if len(sub) < 3 {
			return "\n"
		}
		return hiddenHTMLToken(sub[2])
	})

	text = reCapDownInQuote.ReplaceAllStringFunc(text, func(m string) string {
		sub := reCapDownInQuote.FindStringSubmatch(m)
		if len(sub) < 2 {
			return ""
		}
		return capDownQuoteToken(sub[1])
	})
	text = reCapDownAlone.ReplaceAllStringFunc(text, func(m string) string {
		sub := reCapDownAlone.FindStringSubmatch(m)
		if len(sub) < 2 {
			return ""
		}
		return capDownQuoteToken(sub[1])
	})

	text = rePhotoScroll.ReplaceAllStringFunc(text, func(m string) string {
		sub := rePhotoScroll.FindStringSubmatch(m)
		if len(sub) < 2 {
			return ""
		}
		return galleryToken(sub[1])
	})

	text = reBlurBlock.ReplaceAllStringFunc(text, func(m string) string {
		sub := reBlurBlock.FindStringSubmatch(m)
		if len(sub) < 2 {
			return ""
		}
		return dataBlockToken("BLUR", map[string]any{"content": sub[1]})
	})

	text = reSelfClosing("DottedNumber").ReplaceAllStringFunc(text, func(m string) string {
		attrs := readSelfClosingAttrs(m, "DottedNumber")
		return dataBlockToken("DOTTED_NUMBER", map[string]any{"value": readHTMLAttr(attrs, "n")})
	})

	text = reSelfClosing("TextRing").ReplaceAllStringFunc(text, func(m string) string {
		attrs := readSelfClosingAttrs(m, "TextRing")
		return dataBlockToken("TEXT_RING", map[string]any{
			"text":     readHTMLAttr(attrs, "text"),
			"fontSize": firstNonEmpty(readHTMLAttr(attrs, "fontSize"), readHTMLAttr(attrs, "fontsize")),
		})
	})

	text = reSelfClosing("Sakura").ReplaceAllStringFunc(text, func(m string) string {
		attrs := readSelfClosingAttrs(m, "Sakura")
		return dataBlockToken("SAKURA", map[string]any{"count": readHTMLAttr(attrs, "count")})
	})

	text = reSelfClosing("ChannelBackupButton").ReplaceAllStringFunc(text, func(m string) string {
		attrs := readSelfClosingAttrs(m, "ChannelBackupButton")
		return dataBlockToken("CHANNEL_BACKUP", map[string]any{"platform": readHTMLAttr(attrs, "platform")})
	})

	text = reHexagonOpen.ReplaceAllString(text, "\n\n[[HEXAGON_OPEN]]\n\n")
	text = reHexagonClose.ReplaceAllString(text, "\n\n[[HEXAGON_CLOSE]]\n\n")

	text = reEmptyDiv.ReplaceAllStringFunc(text, func(m string) string {
		sub := reEmptyDiv.FindStringSubmatch(m)
		if len(sub) < 2 {
			return ""
		}
		return emptyDivToken(sub[1])
	})

	text = reDivOpen.ReplaceAllStringFunc(text, func(m string) string {
		sub := reDivOpen.FindStringSubmatch(m)
		if len(sub) < 2 {
			return ""
		}
		return divOpenToken(sub[1])
	})

	text = reDivClose.ReplaceAllString(text, "\n\n[[DIV_CLOSE]]\n\n")
	text = reBR.ReplaceAllString(text, "\n")

	text = reSummary.ReplaceAllStringFunc(text, func(m string) string {
		sub := reSummary.FindStringSubmatch(m)
		if len(sub) < 2 {
			return ""
		}
		return dataBlockToken("SUMMARY", map[string]any{"content": cleanSummaryMarkup(sub[1])})
	})

	text = reDetailsOpen.ReplaceAllString(text, "\n\n[[DETAILS_OPEN]]\n\n")
	text = reDetailsClose.ReplaceAllString(text, "\n\n[[DETAILS_CLOSE]]\n\n")

	text = reHTMLHeading.ReplaceAllStringFunc(text, func(m string) string {
		sub := reHTMLHeading.FindStringSubmatch(m)
		if len(sub) < 4 {
			return ""
		}
		level, _ := strconv.Atoi(sub[1])
		return htmlHeadingToken(level, sub[2], sub[3])
	})

	text = reHTMLParagraph.ReplaceAllStringFunc(text, func(m string) string {
		sub := reHTMLParagraph.FindStringSubmatch(m)
		if len(sub) < 3 {
			return ""
		}
		return htmlParagraphToken(sub[1], sub[2])
	})

	text = reBlockquoteOpen.ReplaceAllString(text, "\n\n[[BLOCKQUOTE_OPEN]]\n\n")
	text = reBlockquoteClose.ReplaceAllString(text, "\n\n[[BLOCKQUOTE_CLOSE]]\n\n")
	text = reUnknownTag.ReplaceAllString(text, "\n")

	return text
}

// --- helpers and regexes ---

var (
	reHTMLComment = regexp.MustCompile(`<!--[\s\S]*?-->`)
	reHiddenTag   = regexp.MustCompile(`(?i)<([a-z][a-z0-9:-]*)\b[^>]*style\s*=\s*["'][^"']*(?:font-size\s*:\s*0(?:\.\d+)?px|display\s*:\s*none|visibility\s*:\s*hidden|opacity\s*:\s*0)[^"']*["'][^>]*>([\s\S]*?)</[^>]+>`)
	reCapDownInQuote = regexp.MustCompile(`(?is)<blockquote>\s*<CapDownQuote\s+messages=\{([\s\S]*?)\}\s*/?>\s*</blockquote>`)
	reCapDownAlone   = regexp.MustCompile(`(?is)<CapDownQuote\s+messages=\{([\s\S]*?)\}\s*/?>`)
	rePhotoScroll    = regexp.MustCompile(`(?is)<PhotoScroll\s+photos=\{(\[[\s\S]*?\])\}\s*/?>`)
	reBlurBlock      = regexp.MustCompile(`(?is)<BlurBlock[^>]*>([\s\S]*?)</BlurBlock>`)
	reHexagonOpen    = regexp.MustCompile(`(?i)<Hexagon\b[^>]*>`)
	reHexagonClose   = regexp.MustCompile(`(?i)</Hexagon>`)
	reEmptyDiv       = regexp.MustCompile(`(?is)<div\b([^>]*)>\s*</div>`)
	reDivOpen        = regexp.MustCompile(`(?i)<div\b([^>]*)>`)
	reDivClose       = regexp.MustCompile(`(?i)</div>`)
	reBR             = regexp.MustCompile(`(?i)<br\s*/?>`)
	reSummary        = regexp.MustCompile(`(?is)<summary>([\s\S]*?)</summary>`)
	reDetailsOpen    = regexp.MustCompile(`(?i)<details[^>]*>`)
	reDetailsClose   = regexp.MustCompile(`(?i)</details>`)
	reHTMLHeading    = regexp.MustCompile(`(?is)<h([1-6])\b([^>]*)>([\s\S]*?)</h[1-6]>`)
	reHTMLParagraph  = regexp.MustCompile(`(?is)<p\b([^>]*)>([\s\S]*?)</p>`)
	reBlockquoteOpen = regexp.MustCompile(`(?i)<blockquote\b[^>]*>`)
	reBlockquoteClose = regexp.MustCompile(`(?i)</blockquote>`)
	reUnknownTag     = regexp.MustCompile(`</?[A-Z][A-Za-z0-9]*(?:\s[^>]*)?/?>`)
)

func reSelfClosing(name string) *regexp.Regexp {
	return regexp.MustCompile(`(?is)<` + name + `\b([^>]*)/?>`)
}

func readSelfClosingAttrs(text, name string) string {
	re := reSelfClosing(name)
	m := re.FindStringSubmatch(text)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func readHTMLAttr(attrs, name string) string {
	pattern := regexp.MustCompile(`(?i)` + name + `\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
	m := pattern.FindStringSubmatch(attrs)
	if len(m) == 0 {
		return ""
	}
	for _, g := range m[1:] {
		if g != "" {
			return strings.TrimSpace(g)
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// galleryToken takes the raw photos={[ ... ]} body and produces a [[GALLERY:...]] block.
func galleryToken(markup string) string {
	re := regexp.MustCompile(`['"]([^'"]+)['"]`)
	matches := re.FindAllStringSubmatch(markup, -1)
	if len(matches) == 0 {
		return "\n"
	}
	parts := make([]string, 0, len(matches))
	for _, m := range matches {
		parts = append(parts, url.QueryEscape(m[1]))
	}
	return fmt.Sprintf("\n\n[[GALLERY:%s]]\n\n", strings.Join(parts, "|"))
}

// capDownQuoteToken parses ['msg', 'msg', ...] style messages from JSX-ish input.
func capDownQuoteToken(messages string) string {
	re := regexp.MustCompile(`['"]([^'"]+)['"]`)
	matches := re.FindAllStringSubmatch(messages, -1)
	if len(matches) == 0 {
		return ""
	}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return dataBlockToken("CAPDOWN_QUOTE", map[string]any{"messages": out})
}

func hiddenHTMLToken(content string) string {
	return fmt.Sprintf("\n\n[[HIDDEN_HTML:%s]]\n\n", url.QueryEscape(strings.TrimSpace(content)))
}

func dataBlockToken(name string, data map[string]any) string {
	return fmt.Sprintf("\n\n[[%s:%s]]\n\n", name, encodeTokenData(data))
}

func dataInlineToken(name string, data map[string]any) string {
	return fmt.Sprintf("[[%s:%s]]", name, encodeTokenData(data))
}

func htmlHeadingToken(level int, attrs, content string) string {
	if level < 1 {
		level = 3
	}
	if level > 6 {
		level = 6
	}
	return dataBlockToken("HTML_HEADING", map[string]any{
		"level":   level,
		"align":   extractTextAlign(attrs),
		"content": cleanHTMLFragment(content),
	})
}

func htmlParagraphToken(attrs, content string) string {
	style := readHTMLAttr(attrs, "style")
	return dataInlineToken("HTML_P", map[string]any{
		"align":   extractTextAlign(attrs),
		"inline":  regexp.MustCompile(`(?i)display\s*:\s*inline`).MatchString(style),
		"content": cleanHTMLFragment(content),
	})
}

func extractTextAlign(attrs string) string {
	style := readHTMLAttr(attrs, "style")
	m := regexp.MustCompile(`(?i)text-align\s*:\s*(start|end|left|right|center)`).FindStringSubmatch(style)
	if len(m) == 2 {
		return normalizeAlign(m[1])
	}
	return normalizeAlign(readHTMLAttr(attrs, "align"))
}

func normalizeAlign(value string) string {
	switch strings.ToLower(value) {
	case "center", "left", "right", "start", "end":
		return strings.ToLower(value)
	}
	return ""
}

func cleanHTMLFragment(value string) string {
	return strings.TrimSpace(value)
}

func cleanSummaryMarkup(value string) string {
	return strings.TrimSpace(value)
}

func emptyDivToken(attrs string) string {
	mode := "block"
	if regexp.MustCompile(`(?i)display\s*:\s*flex`).MatchString(readHTMLAttr(attrs, "style")) {
		mode = "flex"
	}
	return fmt.Sprintf("\n\n[[DIV_OPEN:%s]]\n\n[[DIV_CLOSE]]\n\n", encodeTokenData(map[string]any{"mode": mode}))
}

func divOpenToken(attrs string) string {
	mode := "block"
	if regexp.MustCompile(`(?i)display\s*:\s*flex`).MatchString(readHTMLAttr(attrs, "style")) {
		mode = "flex"
	}
	return fmt.Sprintf("\n\n[[DIV_OPEN:%s]]\n\n", encodeTokenData(map[string]any{"mode": mode}))
}
