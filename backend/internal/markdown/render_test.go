package markdown

import (
	"strings"
	"testing"
)

func TestStripFrontmatter(t *testing.T) {
	in := "---\n名字: 测试\n---\n\n## 简介\n\nhello"
	got := StripFrontmatter(in)
	want := "## 简介\n\nhello"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestRenderHeadings(t *testing.T) {
	got := Render("## 简介\n\n这是 Aki 的故事。", "demo")
	if !strings.Contains(got, "<h3>简介</h3>") {
		t.Errorf("missing heading: %s", got)
	}
	if !strings.Contains(got, "<p>这是 Aki 的故事。</p>") {
		t.Errorf("missing paragraph: %s", got)
	}
}

func TestRenderListsAndQuotes(t *testing.T) {
	got := Render("- 一\n- 二\n\n> hello\n\n1. a\n2. b", "demo")
	for _, frag := range []string{"<ul><li>一</li><li>二</li></ul>", "<blockquote><p>hello</p></blockquote>", "<ol><li>a</li><li>b</li></ol>"} {
		if !strings.Contains(got, frag) {
			t.Errorf("expected %q in %q", frag, got)
		}
	}
}

func TestRenderImageAndLink(t *testing.T) {
	got := Render("![alt](photo.jpg)\n\n[click](https://example.com)", "alex")
	if !strings.Contains(got, `src="/media/memorials/alex/photo.jpg"`) {
		t.Errorf("expected resolved image src: %s", got)
	}
	if !strings.Contains(got, `<a href="https://example.com" target="_blank" rel="noopener">click</a>`) {
		t.Errorf("expected anchor: %s", got)
	}
}

func TestRenderUnsafeLinkBecomesText(t *testing.T) {
	got := Render("[evil](javascript:alert(1))", "x")
	if strings.Contains(got, "<a") {
		t.Errorf("expected javascript: links to render as plain text, got %s", got)
	}
}

func TestRenderRubyAndSpan(t *testing.T) {
	got := Render(`<ruby>过载<rt>Overload</rt></ruby> <span style="color: transparent;">hello</span>`, "x")
	if !strings.Contains(got, `<ruby>过载<rt>Overload</rt></ruby>`) {
		t.Errorf("ruby missing: %s", got)
	}
	if !strings.Contains(got, "color: transparent") {
		t.Errorf("expected sanitized inline style, got %s", got)
	}
}

func TestRenderFootnotes(t *testing.T) {
	in := "正文 [^1]\n\n[^1]: 注释内容"
	got := Render(in, "x")
	if !strings.Contains(got, `id="fn-1"`) || !strings.Contains(got, "注释内容") {
		t.Errorf("footnote section missing: %s", got)
	}
	if !strings.Contains(got, "<sup>[1]</sup>") {
		t.Errorf("footnote ref missing: %s", got)
	}
}

func TestRenderCapDownQuote(t *testing.T) {
	in := "<CapDownQuote messages={['你好','再见']} />"
	got := Render(in, "x")
	if !strings.Contains(got, "story-memory-stream") {
		t.Errorf("expected capdown quote, got %s", got)
	}
	if !strings.Contains(got, "<p>你好</p>") || !strings.Contains(got, "<p>再见</p>") {
		t.Errorf("expected quoted messages, got %s", got)
	}
}

func TestRenderPhotoScroll(t *testing.T) {
	in := "<PhotoScroll photos={['a.jpg', 'b.png']} />"
	got := Render(in, "alex")
	if !strings.Contains(got, "story-gallery") {
		t.Errorf("expected gallery, got %s", got)
	}
	if !strings.Contains(got, "/media/memorials/alex/a.jpg") {
		t.Errorf("expected resolved gallery url, got %s", got)
	}
}

func TestRenderHTMLParagraph(t *testing.T) {
	in := `<p style="text-align: center; color: transparent;">愿你被温柔记住。</p>`
	got := Render(in, "x")
	if !strings.Contains(got, "story-html-paragraph") {
		t.Errorf("expected html paragraph class, got %s", got)
	}
}

func TestRenderDetails(t *testing.T) {
	in := "<details>\n<summary>展开</summary>\n\n详细内容\n\n</details>"
	got := Render(in, "x")
	if !strings.Contains(got, "<details") || !strings.Contains(got, "<summary>展开</summary>") {
		t.Errorf("expected details/summary, got %s", got)
	}
}

func TestRenderHRBoldEm(t *testing.T) {
	got := Render("---\n\n**粗** *斜* `code`", "x")
	for _, frag := range []string{"<hr class=\"story-break\">", "<strong>粗</strong>", "<em>斜</em>", "<code>code</code>"} {
		if !strings.Contains(got, frag) {
			t.Errorf("expected %q in %q", frag, got)
		}
	}
}

func TestRenderHexagon(t *testing.T) {
	in := "<Hexagon>\n\nA section\n\n</Hexagon>"
	got := Render(in, "x")
	if !strings.Contains(got, `<section class="story-hexagon">`) || !strings.Contains(got, "</section>") {
		t.Errorf("expected hexagon section, got %s", got)
	}
}

func TestRenderHiddenHTML(t *testing.T) {
	cases := []string{
		`<span style="font-size: 0px;">秘密</span>`,
		`<div style="display:none">秘密</div>`,
		`<p style="visibility:hidden">秘密</p>`,
		`<span style="opacity: 0;">秘密</span>`,
	}
	for _, in := range cases {
		got := Render(in, "x")
		if !strings.Contains(got, `class="story-hidden-effect"`) {
			t.Errorf("expected hidden marker for %q, got %s", in, got)
		}
		if !strings.Contains(got, "秘密") {
			t.Errorf("expected hidden text preserved for %q, got %s", in, got)
		}
	}
}

func TestRenderDottedNumber(t *testing.T) {
	got := Render(`<DottedNumber n="3" />`, "x")
	if !strings.Contains(got, "story-number-divider") || !strings.Contains(got, "<span>3</span>") {
		t.Errorf("dotted number: %s", got)
	}
	got2 := Render(`<DottedNumber />`, "x")
	if !strings.Contains(got2, "<span>•</span>") {
		t.Errorf("default dotted number: %s", got2)
	}
}

func TestRenderTextRing(t *testing.T) {
	got := Render(`<TextRing text="勿忘我" />`, "x")
	if !strings.Contains(got, "story-text-ring") || !strings.Contains(got, "勿忘我") {
		t.Errorf("text ring: %s", got)
	}
	got2 := Render(`<TextRing />`, "x")
	if !strings.Contains(got2, "✦") {
		t.Errorf("default text ring: %s", got2)
	}
}

func TestRenderSakura(t *testing.T) {
	got := Render(`<Sakura count="8" />`, "x")
	count := strings.Count(got, "<span style=\"--i:")
	if count != 8 {
		t.Errorf("expected 8 petals, got %d in %s", count, got)
	}
	got2 := Render(`<Sakura />`, "x")
	if c := strings.Count(got2, "<span style=\"--i:"); c != 12 {
		t.Errorf("expected 12 default petals, got %d", c)
	}
	got3 := Render(`<Sakura count="100" />`, "x")
	if c := strings.Count(got3, "<span style=\"--i:"); c != 18 {
		t.Errorf("expected clamp to 18, got %d", c)
	}
}

func TestRenderChannelBackup(t *testing.T) {
	got := Render(`<ChannelBackupButton platform="telegram" />`, "x")
	if !strings.Contains(got, "story-backup-chip") || !strings.Contains(got, "telegram") {
		t.Errorf("channel backup: %s", got)
	}
	got2 := Render(`<ChannelBackupButton />`, "x")
	if !strings.Contains(got2, "channel") {
		t.Errorf("default channel: %s", got2)
	}
}

func TestRenderCapDownQuoteOddMessages(t *testing.T) {
	in := "<CapDownQuote messages={['a','b','c']} />"
	got := Render(in, "x")
	pairs := strings.Count(got, `class="story-memory-pair"`)
	if pairs != 2 {
		t.Errorf("expected 2 pair containers (2+1), got %d in %s", pairs, got)
	}
}

func TestRenderDetailsWithNestedContent(t *testing.T) {
	in := `<details>
<summary>展开</summary>

## 二级标题

- 一
- 二

</details>`
	got := Render(in, "x")
	for _, frag := range []string{"<details", "<summary>展开</summary>", "<h3>二级标题</h3>", "<ul><li>一</li><li>二</li></ul>", "</details>"} {
		if !strings.Contains(got, frag) {
			t.Errorf("expected %q in %s", frag, got)
		}
	}
}

func TestRenderDivWithDisplayFlex(t *testing.T) {
	in := `<div style="display: flex; gap: 1rem;">

inside

</div>`
	got := Render(in, "x")
	if !strings.Contains(got, `class="story-flex-cluster"`) {
		t.Errorf("expected flex container, got %s", got)
	}
	if !strings.Contains(got, "<p>inside</p>") {
		t.Errorf("expected nested paragraph, got %s", got)
	}
}

func TestRenderEmptyDiv(t *testing.T) {
	in := `<div></div>`
	got := Render(in, "x")
	if !strings.Contains(got, "story-html-container") {
		t.Errorf("expected empty div token: %s", got)
	}
}

func TestRenderSpanGradientStyle(t *testing.T) {
	in := `<span style="background: linear-gradient(90deg, #5bcefa, #f5a9b8); background-clip: text; color: transparent;">渐变</span>`
	got := Render(in, "x")
	if !strings.Contains(got, "story-inline-style") {
		t.Errorf("expected inline style span: %s", got)
	}
	if !strings.Contains(got, "linear-gradient(") {
		t.Errorf("expected gradient preserved: %s", got)
	}
	if !strings.Contains(got, "color: transparent") {
		t.Errorf("expected color transparent: %s", got)
	}
}

func TestRenderImageWithPathPlaceholder(t *testing.T) {
	in := "![alt](${path}/photo.jpg)"
	got := Render(in, "alex")
	if !strings.Contains(got, `src="/media/memorials/alex/photo.jpg"`) {
		t.Errorf("expected ${path} replacement, got %s", got)
	}
}

func TestRenderImageRejectsTraversal(t *testing.T) {
	in := "![alt](../../etc/passwd)"
	got := Render(in, "alex")
	if strings.Contains(got, "<img") {
		t.Errorf("expected traversal to be rejected, got %s", got)
	}
}

func TestRenderRubyInsideParagraph(t *testing.T) {
	in := "正文 <ruby>过载<rt>Overload</rt></ruby> 结束"
	got := Render(in, "x")
	if !strings.Contains(got, "<ruby>过载<rt>Overload</rt></ruby>") {
		t.Errorf("expected ruby preserved, got %s", got)
	}
}

func TestRenderSummaryWithMarkdown(t *testing.T) {
	in := `<details>
<summary>**重点**</summary>

正文

</details>`
	got := Render(in, "x")
	if !strings.Contains(got, "<summary>") {
		t.Errorf("expected summary, got %s", got)
	}
	if !strings.Contains(got, "<strong>重点</strong>") {
		t.Errorf("expected bold inside summary, got %s", got)
	}
}

func TestRenderHTMLHeadingLevels(t *testing.T) {
	in := `<h2>大标题</h2>
<h6 align="center">小标题</h6>`
	got := Render(in, "x")
	if !strings.Contains(got, "<h2 ") || !strings.Contains(got, "大标题") {
		t.Errorf("expected h2, got %s", got)
	}
	if !strings.Contains(got, "<h6 ") || !strings.Contains(got, "story-align-center") {
		t.Errorf("expected centered h6, got %s", got)
	}
}

func TestRenderMultipleParagraphs(t *testing.T) {
	in := "第一段第一行\n第一段第二行\n\n第二段"
	got := Render(in, "x")
	pCount := strings.Count(got, "<p>")
	if pCount != 2 {
		t.Errorf("expected 2 <p>, got %d in %s", pCount, got)
	}
	if !strings.Contains(got, "第一段第一行<br>第一段第二行") {
		t.Errorf("expected newlines as <br>, got %s", got)
	}
}

func TestRenderBlockquoteWithMarkdown(t *testing.T) {
	in := "> **强调** 内容\n> 第二行"
	got := Render(in, "x")
	if !strings.Contains(got, "<blockquote>") {
		t.Errorf("expected blockquote, got %s", got)
	}
	if !strings.Contains(got, "<strong>强调</strong>") {
		t.Errorf("expected inline markdown inside blockquote, got %s", got)
	}
}

func TestStripFrontmatterIdempotent(t *testing.T) {
	bare := "## 简介\n\nhello"
	if got := StripFrontmatter(bare); got != bare {
		t.Errorf("expected idempotent on already-stripped input, got %q", got)
	}
}
