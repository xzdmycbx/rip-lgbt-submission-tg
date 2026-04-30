package markdown

import (
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// renderLines mirrors renderMarkdownLines() in frontend.js. It walks the
// preprocessed line stream and produces HTML fragments with paragraph
// flushing whenever a block element is encountered.
func renderLines(lines []string, personPath string) string {
	var b strings.Builder
	var paragraph []string

	flush := func() {
		if len(paragraph) == 0 {
			return
		}
		joined := strings.Join(paragraph, "\n")
		b.WriteString("<p>")
		b.WriteString(strings.ReplaceAll(renderInline(joined, personPath), "\n", "<br>"))
		b.WriteString("</p>")
		paragraph = paragraph[:0]
	}

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			flush()
			continue
		}

		switch trimmed {
		case "[[DETAILS_OPEN]]":
			flush()
			b.WriteString(`<details class="story-details">`)
			continue
		case "[[DETAILS_CLOSE]]":
			flush()
			b.WriteString(`</details>`)
			continue
		case "[[HEXAGON_OPEN]]":
			flush()
			b.WriteString(`<section class="story-hexagon">`)
			continue
		case "[[HEXAGON_CLOSE]]":
			flush()
			b.WriteString(`</section>`)
			continue
		case "[[BLOCKQUOTE_OPEN]]":
			flush()
			b.WriteString(`<blockquote class="story-html-quote">`)
			continue
		case "[[BLOCKQUOTE_CLOSE]]":
			flush()
			b.WriteString(`</blockquote>`)
			continue
		case "[[DIV_CLOSE]]":
			flush()
			b.WriteString(`</div>`)
			continue
		}

		if v, ok := matchToken(trimmed, "DIV_OPEN"); ok {
			flush()
			data := decodeTokenData(v)
			cls := "story-html-container"
			if tokenString(data, "mode") == "flex" {
				cls = "story-flex-cluster"
			}
			fmt.Fprintf(&b, `<div class="%s">`, cls)
			continue
		}

		if v, ok := matchToken(trimmed, "SUMMARY"); ok {
			flush()
			data := decodeTokenData(v)
			fmt.Fprintf(&b, `<summary>%s</summary>`,
				strings.ReplaceAll(renderInline(tokenString(data, "content"), personPath), "\n", "<br>"))
			continue
		}

		if v, ok := matchToken(trimmed, "GALLERY"); ok {
			flush()
			b.WriteString(renderGallery(v, personPath))
			continue
		}

		if v, ok := matchToken(trimmed, "HIDDEN_HTML"); ok {
			flush()
			decoded, _ := url.QueryUnescape(v)
			fmt.Fprintf(&b, `<p class="story-hidden-effect" aria-hidden="true">%s</p>`, renderInline(decoded, personPath))
			continue
		}

		if v, ok := matchToken(trimmed, "HTML_HEADING"); ok {
			flush()
			b.WriteString(renderHTMLHeading(v, personPath))
			continue
		}

		if v, ok := matchToken(trimmed, "HTML_P"); ok {
			flush()
			b.WriteString(renderHTMLParagraph(v, personPath))
			continue
		}

		if v, ok := matchToken(trimmed, "DOTTED_NUMBER"); ok {
			flush()
			b.WriteString(renderDottedNumber(v))
			continue
		}

		if v, ok := matchToken(trimmed, "TEXT_RING"); ok {
			flush()
			b.WriteString(renderTextRing(v))
			continue
		}

		if v, ok := matchToken(trimmed, "BLUR"); ok {
			flush()
			b.WriteString(renderBlurBlock(v, personPath))
			continue
		}

		if v, ok := matchToken(trimmed, "SAKURA"); ok {
			flush()
			b.WriteString(renderSakura(v))
			continue
		}

		if v, ok := matchToken(trimmed, "CHANNEL_BACKUP"); ok {
			flush()
			b.WriteString(renderChannelBackup(v))
			continue
		}

		if v, ok := matchToken(trimmed, "CAPDOWN_QUOTE"); ok {
			flush()
			b.WriteString(renderCapDownQuote(v, personPath))
			continue
		}

		if v, ok := matchToken(trimmed, "SPACER"); ok {
			flush()
			b.WriteString(renderSpacer(v))
			continue
		}

		if m := reHeading.FindStringSubmatch(trimmed); m != nil {
			flush()
			level := len(m[1]) + 1
			if level > 6 {
				level = 6
			}
			fmt.Fprintf(&b, `<h%d>%s</h%d>`, level, renderInline(m[2], personPath), level)
			continue
		}

		if reBlockquote.MatchString(trimmed) {
			flush()
			var quote []string
			for i < len(lines) && reBlockquote.MatchString(strings.TrimSpace(lines[i])) {
				quote = append(quote, reBlockquote.ReplaceAllString(strings.TrimSpace(lines[i]), ""))
				i++
			}
			i--
			b.WriteString(`<blockquote>`)
			for _, q := range quote {
				fmt.Fprintf(&b, `<p>%s</p>`, renderInline(q, personPath))
			}
			b.WriteString(`</blockquote>`)
			continue
		}

		if reUL.MatchString(trimmed) {
			flush()
			var items []string
			for i < len(lines) && reUL.MatchString(strings.TrimSpace(lines[i])) {
				items = append(items, reUL.ReplaceAllString(strings.TrimSpace(lines[i]), ""))
				i++
			}
			i--
			b.WriteString(`<ul>`)
			for _, item := range items {
				fmt.Fprintf(&b, `<li>%s</li>`, renderInline(item, personPath))
			}
			b.WriteString(`</ul>`)
			continue
		}

		if reOL.MatchString(trimmed) {
			flush()
			var items []string
			for i < len(lines) && reOL.MatchString(strings.TrimSpace(lines[i])) {
				items = append(items, reOL.ReplaceAllString(strings.TrimSpace(lines[i]), ""))
				i++
			}
			i--
			b.WriteString(`<ol>`)
			for _, item := range items {
				fmt.Fprintf(&b, `<li>%s</li>`, renderInline(item, personPath))
			}
			b.WriteString(`</ol>`)
			continue
		}

		if reHR.MatchString(trimmed) {
			flush()
			b.WriteString(`<hr class="story-break">`)
			continue
		}

		paragraph = append(paragraph, trimmed)
	}

	flush()
	return b.String()
}

var (
	reHeading    = regexp.MustCompile(`^(#{2,5})\s+(.+)$`)
	reBlockquote = regexp.MustCompile(`^>\s?`)
	reUL         = regexp.MustCompile(`^[-*]\s+`)
	reOL         = regexp.MustCompile(`^\d+\.\s+`)
	reHR         = regexp.MustCompile(`^-{3,}$`)
)

// matchToken returns the inner payload if the line is a [[NAME:payload]] block.
func matchToken(line, name string) (string, bool) {
	prefix := "[[" + name + ":"
	if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, "]]") {
		return "", false
	}
	return line[len(prefix) : len(line)-2], true
}

// --- block renderers ---

func renderGallery(value, personPath string) string {
	parts := strings.Split(value, "|")
	var urls []string
	for _, p := range parts {
		decoded, err := url.QueryUnescape(p)
		if err != nil {
			continue
		}
		if asset := toContentAssetURL(decoded, personPath); asset != "" {
			urls = append(urls, asset)
		}
	}
	if len(urls) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="story-gallery">`)
	for _, u := range urls {
		fmt.Fprintf(&b, `<img src="%s" alt="" loading="lazy" decoding="async">`, escapeAttr(u))
	}
	b.WriteString(`</div>`)
	return b.String()
}

func renderHTMLHeading(value, personPath string) string {
	data := decodeTokenData(value)
	level, _ := strconv.Atoi(tokenString(data, "level"))
	if level < 1 {
		level = 3
	}
	if level > 6 {
		level = 6
	}
	align := tokenString(data, "align")
	cls := "story-html-heading"
	if align != "" {
		cls += " story-align-" + align
	}
	return fmt.Sprintf(`<h%d class="%s">%s</h%d>`, level, escapeAttr(cls),
		strings.ReplaceAll(renderInline(tokenString(data, "content"), personPath), "\n", "<br>"), level)
}

func renderHTMLParagraph(value, personPath string) string {
	data := decodeTokenData(value)
	classes := []string{"story-html-paragraph"}
	if a := tokenString(data, "align"); a != "" {
		classes = append(classes, "story-align-"+a)
	}
	if tokenBool(data, "inline") {
		classes = append(classes, "story-inline-paragraph")
	}
	return fmt.Sprintf(`<p class="%s">%s</p>`, escapeAttr(strings.Join(classes, " ")),
		strings.ReplaceAll(renderInline(tokenString(data, "content"), personPath), "\n", "<br>"))
}

func renderDottedNumber(value string) string {
	data := decodeTokenData(value)
	v := tokenString(data, "value")
	if v == "" {
		v = "•"
	}
	return fmt.Sprintf(`<div class="story-number-divider" aria-hidden="true"><span>%s</span></div>`, escapeHTML(v))
}

func renderTextRing(value string) string {
	data := decodeTokenData(value)
	t := tokenString(data, "text")
	if t == "" {
		t = "✦"
	}
	return fmt.Sprintf(`<div class="story-text-ring" aria-hidden="true">%s</div>`, escapeHTML(t))
}

func renderBlurBlock(value, personPath string) string {
	data := decodeTokenData(value)
	return fmt.Sprintf(`<p class="story-blur-block" tabindex="0">%s</p>`,
		strings.ReplaceAll(renderInline(tokenString(data, "content"), personPath), "\n", "<br>"))
}

func renderSakura(value string) string {
	data := decodeTokenData(value)
	count, _ := strconv.Atoi(tokenString(data, "count"))
	if count < 6 {
		count = 12
	}
	if count > 18 {
		count = 18
	}
	var b strings.Builder
	b.WriteString(`<div class="story-sakura-field" aria-hidden="true">`)
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, `<span style="--i:%d">✦</span>`, i)
	}
	b.WriteString(`</div>`)
	return b.String()
}

func renderChannelBackup(value string) string {
	data := decodeTokenData(value)
	platform := tokenString(data, "platform")
	if platform == "" {
		platform = "channel"
	}
	return fmt.Sprintf(`<aside class="story-backup-chip"><span>%s</span><strong>频道备份</strong></aside>`, escapeHTML(platform))
}

func renderCapDownQuote(value, personPath string) string {
	data := decodeTokenData(value)
	messages := tokenStringSlice(data, "messages")
	if len(messages) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<blockquote class="story-memory-stream">`)
	for i := 0; i < len(messages); i += 2 {
		end := i + 2
		if end > len(messages) {
			end = len(messages)
		}
		b.WriteString(`<div class="story-memory-pair">`)
		for _, msg := range messages[i:end] {
			fmt.Fprintf(&b, `<p>%s</p>`, renderInline(msg, personPath))
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</blockquote>`)
	return b.String()
}

func renderSpacer(value string) string {
	data := decodeTokenData(value)
	cls := "story-spacer"
	if tokenString(data, "size") == "large" {
		cls += " story-spacer-large"
	}
	return fmt.Sprintf(`<div class="%s" aria-hidden="true"></div>`, escapeAttr(cls))
}

// --- shared utilities ---

func toContentAssetURL(value, personPath string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	root := "/media/memorials/" + url.PathEscape(personPath)
	value = strings.ReplaceAll(value, "${path}", root)
	if isSafeURL(value) || strings.HasPrefix(value, "/") {
		return value
	}
	if regexp.MustCompile(`(?i)^[\w./%() -]+\.(?:avif|gif|jpe?g|png|svg|webp)$`).MatchString(value) && !strings.Contains(value, "..") {
		clean := strings.TrimPrefix(value, "./")
		clean = strings.TrimPrefix(clean, "/")
		return root + "/" + clean
	}
	return ""
}

func isSafeURL(value string) bool {
	if strings.HasPrefix(strings.ToLower(value), "http://") {
		return true
	}
	if strings.HasPrefix(strings.ToLower(value), "https://") {
		return true
	}
	return false
}

// escapeHTML mirrors frontend.js escapeHtml; preserves apostrophe-escape style.
func escapeHTML(value string) string {
	value = html.EscapeString(value)
	// Go's escaper uses &#34; / &#39;; the JS one used &quot; / &#39;.
	value = strings.ReplaceAll(value, "&#34;", "&quot;")
	return value
}

// escapeAttr is the same routine used for HTML attribute values.
func escapeAttr(value string) string { return escapeHTML(value) }
