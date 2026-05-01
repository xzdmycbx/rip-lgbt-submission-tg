package memorial

import (
	"fmt"
	"strings"
)

// GenerateMarkdown rebuilds the canonical published markdown from the
// structured fields. Both the admin edit form and the draft promotion
// path call this so memorials always have an up-to-date markdown_full
// without anyone hand-editing it.
//
// The shape stays close to template.md but only includes sections that
// actually have content.
func GenerateMarkdown(m *AdminMemorial) string {
	var b strings.Builder
	if m.DisplayName != "" {
		fmt.Fprintf(&b, "# %s\n\n", m.DisplayName)
	}
	if m.Description != "" {
		fmt.Fprintf(&b, "> %s\n\n", m.Description)
	}

	for _, p := range []struct{ Title, Body string }{
		{"简介", m.Intro},
		{"生平与记忆", m.Life},
		{"离世", m.Death},
		{"念想", m.Remembrance},
		{"作品", m.WorksMD},
		{"公开链接", m.LinksMD},
		{"资料来源", m.SourcesMD},
		{"自选附加项", m.CustomMD},
		{"排版与特殊效果", m.EffectsMD},
	} {
		if strings.TrimSpace(p.Body) == "" {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", p.Title, strings.TrimSpace(p.Body))
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// GenerateFacts derives the public-info table rows. Empty values are
// dropped so the detail page doesn't render bullets like "昵称：" with
// nothing after them.
func GenerateFacts(m *AdminMemorial) []Fact {
	pairs := []struct{ Label, Value string }{
		{"地区", m.Location},
		{"出生日期", m.BirthDate},
		{"逝世日期", m.DeathDate},
		{"昵称", m.Alias},
		{"年龄", m.Age},
		{"身份表述", m.Identity},
		{"代词", m.Pronouns},
	}
	out := make([]Fact, 0, len(pairs))
	for _, p := range pairs {
		v := strings.TrimSpace(p.Value)
		if v == "" {
			continue
		}
		out = append(out, Fact{Label: p.Label, Value: v})
	}
	return out
}

// GenerateWebsites parses the textarea-style links_md into a structured
// list of {label, url} pairs. Lines without a recognizable URL are
// dropped silently so only valid links surface on the detail page.
func GenerateWebsites(linksMD string) []Site {
	out := []Site{}
	for _, line := range strings.Split(linksMD, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var label, url string
		switch {
		case strings.Contains(line, " - "):
			parts := strings.SplitN(line, " - ", 2)
			label, url = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		case strings.Contains(line, "://"):
			// e.g. "twitter: https://x.com/u" — split on the FIRST ": " so
			// the URL's :// stays intact.
			if i := strings.Index(line, ": "); i > 0 {
				label = strings.TrimSpace(line[:i])
				url = strings.TrimSpace(line[i+2:])
			} else {
				url = line
				label = "链接"
			}
		default:
			continue
		}
		if !strings.HasPrefix(strings.ToLower(url), "http://") && !strings.HasPrefix(strings.ToLower(url), "https://") {
			continue
		}
		if label == "" {
			label = "链接"
		}
		out = append(out, Site{Label: label, URL: url})
	}
	return out
}
