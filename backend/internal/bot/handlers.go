package bot

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
	"github.com/google/uuid"
	"os"

	"github.com/ripyc/rip-lgbt-submission-tg/internal/submission"
)

func (m *Manager) registerHandlers(d *ext.Dispatcher) {
	d.AddHandler(handlers.NewCommand("start", m.handleStart))
	d.AddHandler(handlers.NewCommand("submit", m.handleSubmit))
	d.AddHandler(handlers.NewCommand("cancel", m.handleCancel))
	d.AddHandler(handlers.NewCommand("login", m.handleLogin))

	d.AddHandler(handlers.NewCallback(callbackPrefix("step:"), m.handleStepCallback))
	d.AddHandler(handlers.NewCallback(callbackPrefix("nav:"), m.handleNavCallback))
	d.AddHandler(handlers.NewCallback(callbackPrefix("submit"), m.handleSubmitFinal))

	d.AddHandler(handlers.NewMessage(message.Photo, m.handlePhoto))
	d.AddHandler(handlers.NewMessage(message.Text, m.handleText))
}

func callbackPrefix(prefix string) func(cq *gotgbot.CallbackQuery) bool {
	return func(cq *gotgbot.CallbackQuery) bool {
		return cq != nil && strings.HasPrefix(cq.Data, prefix)
	}
}

// --- /start ---

func (m *Manager) handleStart(b *gotgbot.Bot, ctx *ext.Context) error {
	chat := ctx.EffectiveChat
	user := ctx.EffectiveUser
	if chat == nil || user == nil {
		return nil
	}
	text := fmt.Sprintf(
		"你好，%s。\n\n这里是 rip.lgbt 的投稿机器人。\n输入 /submit 开始一份新的投稿，机器人会一步一步带你填写。\n\n你也可以随时输入 /cancel 取消正在进行的投稿。",
		safeName(user))
	_, err := b.SendMessage(chat.Id, text, nil)
	return err
}

// --- /submit ---

func (m *Manager) handleSubmit(b *gotgbot.Bot, ctx *ext.Context) error {
	chat := ctx.EffectiveChat
	user := ctx.EffectiveUser
	if chat == nil || user == nil {
		return nil
	}
	c := context.Background()

	d, err := m.drafts.FindOpenByTelegram(c, user.Id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if d == nil {
		d, err = m.drafts.Create(c, user.Id, chat.Id)
		if err != nil {
			return err
		}
	}
	step := submission.Steps()[0]
	d.CurrentStep = step.Key
	if err := m.drafts.Save(c, d); err != nil {
		return err
	}
	return m.sendStepMessage(b, c, d, step, true)
}

// --- /cancel ---

func (m *Manager) handleCancel(b *gotgbot.Bot, ctx *ext.Context) error {
	chat := ctx.EffectiveChat
	user := ctx.EffectiveUser
	if chat == nil || user == nil {
		return nil
	}
	c := context.Background()
	d, err := m.drafts.FindOpenByTelegram(c, user.Id)
	if err != nil || d == nil {
		_, _ = b.SendMessage(chat.Id, "当前没有正在进行的投稿。", nil)
		return nil
	}
	if err := m.drafts.SoftDelete(c, d.ID); err != nil {
		return err
	}
	_, _ = b.SendMessage(chat.Id, "已取消当前投稿。输入 /submit 可以重新开始。", nil)
	return nil
}

// --- /login (TG admin one-shot login) ---

func (m *Manager) handleLogin(b *gotgbot.Bot, ctx *ext.Context) error {
	chat := ctx.EffectiveChat
	user := ctx.EffectiveUser
	if chat == nil || user == nil {
		return nil
	}
	c := context.Background()
	admin, err := m.authStore.GetAdminByTelegramID(c, user.Id)
	if errors.Is(err, sql.ErrNoRows) || admin == nil {
		_, _ = b.SendMessage(chat.Id, "你不是管理员，无法生成登录链接。", nil)
		return nil
	}
	if err != nil {
		return err
	}
	url, err := m.authService.IssueLoginLink(c, admin.ID)
	if err != nil {
		return err
	}
	_, _ = b.SendMessage(chat.Id,
		fmt.Sprintf("登录链接（10 分钟内有效，仅可使用一次）：\n%s", url), nil)
	return nil
}

// --- text input ---

func (m *Manager) handleText(b *gotgbot.Bot, ctx *ext.Context) error {
	chat := ctx.EffectiveChat
	user := ctx.EffectiveUser
	msg := ctx.EffectiveMessage
	if chat == nil || user == nil || msg == nil {
		return nil
	}
	if strings.HasPrefix(msg.Text, "/") {
		return nil
	}
	c := context.Background()
	d, err := m.drafts.FindOpenByTelegram(c, user.Id)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if d == nil {
		_, _ = b.SendMessage(chat.Id, "没有正在进行的投稿。输入 /submit 开始一份新的投稿。", nil)
		return nil
	}
	step, ok := submission.FindStep(d.CurrentStep)
	if !ok {
		step = submission.Steps()[0]
	}

	// Always delete the user's message ASAP so private content stays minimal.
	if _, err := b.DeleteMessage(chat.Id, msg.MessageId, nil); err != nil {
		m.logger.Warn("delete user message", "err", err)
	}

	switch step.Kind {
	case submission.StepText, submission.StepShortText:
		// Validate entry_id immediately so the user is not surprised at
		// submission time. Other free-form fields are unrestricted.
		if step.Key == "entry_id" {
			status, err := submission.CheckEntryID(c, m.drafts.DB, msg.Text, d.ID)
			if err != nil {
				m.logger.Warn("check entry id", "err", err)
			}
			if status != submission.EntryIDOK {
				// Stay on the same step, surface the message via a
				// transient toast-style note in the prompt.
				note := submission.EntryIDStatusMessage(status)
				_ = m.sendStepWithNote(b, c, d, step, note)
				return nil
			}
		}
		d.SetStringField(step.Key, msg.Text)
	case submission.StepImage:
		// Allow user to type "none" to skip avatars
		if strings.EqualFold(strings.TrimSpace(msg.Text), "none") {
			d.SetStringField(step.Key, "none")
		} else {
			// ignore: user must send a picture
			_ = m.sendStepMessage(b, c, d, step, false)
			return nil
		}
	case submission.StepImages:
		// noop — user should press 下一步 when done
	}
	if err := m.drafts.Save(c, d); err != nil {
		return err
	}
	next := submission.NextStep(step.Key)
	d.CurrentStep = next.Key
	if err := m.drafts.Save(c, d); err != nil {
		return err
	}
	return m.sendStepMessage(b, c, d, next, false)
}

// --- photo input ---

func (m *Manager) handlePhoto(b *gotgbot.Bot, ctx *ext.Context) error {
	chat := ctx.EffectiveChat
	user := ctx.EffectiveUser
	msg := ctx.EffectiveMessage
	if chat == nil || user == nil || msg == nil {
		return nil
	}
	c := context.Background()
	d, err := m.drafts.FindOpenByTelegram(c, user.Id)
	if err != nil || d == nil {
		_, _ = b.SendMessage(chat.Id, "请先输入 /submit 开始投稿。", nil)
		return nil
	}
	step, _ := submission.FindStep(d.CurrentStep)
	if step.Kind != submission.StepImage && step.Kind != submission.StepImages {
		_ = m.sendStepMessage(b, c, d, step, false)
		return nil
	}

	largest := pickLargestPhoto(msg.Photo)
	if largest == nil {
		return nil
	}
	if err := m.downloadPhotoToDraft(b, d, step.AssetRole, largest); err != nil {
		m.logger.Warn("download photo", "err", err)
		_, _ = b.SendMessage(chat.Id, "图片保存失败，请重试。", nil)
		return nil
	}
	if _, err := b.DeleteMessage(chat.Id, msg.MessageId, nil); err != nil {
		m.logger.Warn("delete user photo", "err", err)
	}
	if step.Kind == submission.StepImage {
		// move forward automatically
		next := submission.NextStep(step.Key)
		d.CurrentStep = next.Key
		_ = m.drafts.Save(c, d)
		return m.sendStepMessage(b, c, d, next, false)
	}
	// images — keep collecting; refresh the step prompt to show count
	return m.sendStepMessage(b, c, d, step, false)
}

func pickLargestPhoto(photos []gotgbot.PhotoSize) *gotgbot.PhotoSize {
	var best *gotgbot.PhotoSize
	for i, p := range photos {
		if best == nil || p.FileSize > best.FileSize {
			best = &photos[i]
		}
	}
	return best
}

func (m *Manager) downloadPhotoToDraft(b *gotgbot.Bot, d *submission.Draft, role string, p *gotgbot.PhotoSize) error {
	file, err := b.GetFile(p.FileId, nil)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", b.Token, file.FilePath)
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("download status %d", resp.StatusCode)
	}
	dir := m.drafts.DraftDir(d.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := fmt.Sprintf("%s_%s.jpg", role, uuid.NewString()[:8])
	target := filepath.Join(dir, name)
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	n, err := io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		return err
	}
	if _, err := m.drafts.AddAsset(context.Background(), d.ID, role, name, "image/jpeg", n); err != nil {
		return err
	}
	return nil
}

// --- callbacks ---

func (m *Manager) handleStepCallback(b *gotgbot.Bot, ctx *ext.Context) error {
	cq := ctx.CallbackQuery
	if cq == nil {
		return nil
	}
	target := strings.TrimPrefix(cq.Data, "step:")
	step, ok := submission.FindStep(target)
	if !ok {
		_, _ = cq.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "未知步骤"})
		return nil
	}
	c := context.Background()
	d, err := m.drafts.FindOpenByTelegram(c, cq.From.Id)
	if err != nil || d == nil {
		_, _ = cq.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "请先输入 /submit"})
		return nil
	}
	d.CurrentStep = step.Key
	_ = m.drafts.Save(c, d)
	_, _ = cq.Answer(b, nil)
	return m.sendStepMessage(b, c, d, step, false)
}

func (m *Manager) handleNavCallback(b *gotgbot.Bot, ctx *ext.Context) error {
	cq := ctx.CallbackQuery
	if cq == nil {
		return nil
	}
	dir := strings.TrimPrefix(cq.Data, "nav:")
	c := context.Background()
	d, err := m.drafts.FindOpenByTelegram(c, cq.From.Id)
	if err != nil || d == nil {
		_, _ = cq.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "请先输入 /submit"})
		return nil
	}
	current, _ := submission.FindStep(d.CurrentStep)
	var next submission.Step
	switch dir {
	case "prev":
		next = submission.PrevStep(current.Key)
	case "next", "skip":
		next = submission.NextStep(current.Key)
	case "menu":
		_, _ = cq.Answer(b, nil)
		return m.sendStepMenu(b, c, d, current)
	default:
		_, _ = cq.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "未知操作"})
		return nil
	}
	d.CurrentStep = next.Key
	_ = m.drafts.Save(c, d)
	_, _ = cq.Answer(b, nil)
	return m.sendStepMessage(b, c, d, next, false)
}

func (m *Manager) handleSubmitFinal(b *gotgbot.Bot, ctx *ext.Context) error {
	cq := ctx.CallbackQuery
	if cq == nil {
		return nil
	}
	c := context.Background()
	d, err := m.drafts.FindOpenByTelegram(c, cq.From.Id)
	if err != nil || d == nil {
		_, _ = cq.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "没有可提交的投稿"})
		return nil
	}
	missing := requiredMissing(d)
	if len(missing) > 0 {
		_, _ = cq.Answer(b, &gotgbot.AnswerCallbackQueryOpts{
			Text:      "尚有必填字段未完成：" + strings.Join(missing, "、"),
			ShowAlert: true,
		})
		return nil
	}
	d.Status = submission.StatusReview
	d.CurrentStep = ""
	if err := m.drafts.Save(c, d); err != nil {
		return err
	}
	_, _ = cq.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "已提交审核"})
	_, _ = b.SendMessage(cq.From.Id,
		"已提交审核。管理员看到投稿后会给你反馈。如果需要修改某一节，机器人会再次告诉你。", nil)
	go m.notifyAdminsOfNewDraft(d.ID)
	return nil
}

// --- prompts and keyboards ---

// sendStepMessage renders the prompt for `step` and either edits the
// existing main message or sends a fresh one when no main message exists
// yet (or after `fresh` is true to force a new send).
func (m *Manager) sendStepMessage(b *gotgbot.Bot, ctx context.Context, d *submission.Draft, step submission.Step, fresh bool) error {
	return m.sendStepWithNote(b, ctx, d, step, "")
}

// sendStepWithNote is the same as sendStepMessage but injects a one-shot
// note (typically a validation message) above the regular prompt.
func (m *Manager) sendStepWithNote(b *gotgbot.Bot, ctx context.Context, d *submission.Draft, step submission.Step, note string) error {
	text := renderStepText(d, step)
	if note != "" {
		text = "⚠️ " + htmlEscape(note) + "\n\n" + text
	}
	kb := buildStepKeyboard(d, step)
	chatID, msgID, err := m.drafts.LatestMainMessage(ctx, d.ID)
	if err != nil {
		return err
	}
	if chatID != 0 && msgID != 0 {
		_, _, errEdit := b.EditMessageText(text, &gotgbot.EditMessageTextOpts{
			ChatId:      chatID,
			MessageId:   msgID,
			ReplyMarkup: kb,
			ParseMode:   "HTML",
		})
		if errEdit == nil {
			return nil
		}
	}
	sent, err := b.SendMessage(d.SubmitterChatID, text, &gotgbot.SendMessageOpts{
		ReplyMarkup: kb,
		ParseMode:   "HTML",
	})
	if err != nil {
		return err
	}
	return m.drafts.ReplaceMainMessage(ctx, d.ID, d.SubmitterChatID, sent.MessageId)
}

func renderStepText(d *submission.Draft, step submission.Step) string {
	progress := stepProgress(d.CurrentStep)
	var b strings.Builder
	fmt.Fprintf(&b, "<b>%s</b>  <i>(%s)</i>\n\n", htmlEscape(step.Title), progress)
	if step.Kind == submission.StepFinal {
		b.WriteString(htmlEscape(step.Prompt))
		b.WriteString("\n\n")
		b.WriteString(renderDraftSummary(d))
	} else {
		b.WriteString(htmlEscape(step.Prompt))
		if step.Example != "" {
			b.WriteString("\n\n")
			b.WriteString(htmlEscape(step.Example))
		}
		if v := d.GetString(step.Key); v != "" && step.Kind != submission.StepImage {
			fmt.Fprintf(&b, "\n\n<u>当前已填</u>\n<i>%s</i>", htmlEscape(truncate(v, 320)))
		}
	}
	return b.String()
}

// renderDraftSummary lays out everything the user has filled so far in a
// compact preview the user can scan before pressing 提交审核.
func renderDraftSummary(d *submission.Draft) string {
	var b strings.Builder
	b.WriteString("<b>📋 当前内容预览</b>\n")
	for _, step := range submission.Steps() {
		if step.Kind == submission.StepFinal {
			continue
		}
		v := d.GetString(step.Key)
		if step.Kind == submission.StepImage || step.Kind == submission.StepImages {
			n := 0
			for _, a := range d.Assets {
				if a.Role == step.AssetRole {
					n++
				}
			}
			if n > 0 {
				fmt.Fprintf(&b, "✓ %s · %d 张图\n", htmlEscape(step.Title), n)
				continue
			}
			if v == "none" {
				fmt.Fprintf(&b, "○ %s · 标记为无\n", htmlEscape(step.Title))
				continue
			}
			if step.Required {
				fmt.Fprintf(&b, "✗ %s · <i>未上传</i>\n", htmlEscape(step.Title))
			} else {
				fmt.Fprintf(&b, "○ %s · 跳过\n", htmlEscape(step.Title))
			}
			continue
		}
		if v == "" {
			if step.Required {
				fmt.Fprintf(&b, "✗ %s · <i>未填写</i>\n", htmlEscape(step.Title))
			}
			continue
		}
		fmt.Fprintf(&b, "✓ %s · %s\n", htmlEscape(step.Title), htmlEscape(truncate(v, 60)))
	}
	b.WriteString("\n点击下方按钮可跳到任意一节修改，确认无误后再提交。")
	return b.String()
}

func stepProgress(key string) string {
	steps := submission.Steps()
	for i, s := range steps {
		if s.Key == key {
			return fmt.Sprintf("第 %d / %d 步", i+1, len(steps))
		}
	}
	return ""
}

func buildStepKeyboard(d *submission.Draft, step submission.Step) gotgbot.InlineKeyboardMarkup {
	rows := [][]gotgbot.InlineKeyboardButton{}
	if step.Kind == submission.StepFinal {
		// At the review step the user can still freely jump to any
		// section, then come back here to submit.
		rows = append(rows, []gotgbot.InlineKeyboardButton{
			{Text: "📋 跳到任意步骤修改", CallbackData: "nav:menu"},
		})
		rows = append(rows, []gotgbot.InlineKeyboardButton{
			{Text: "◀ 上一步", CallbackData: "nav:prev"},
			{Text: "✅ 提交审核", CallbackData: "submit"},
		})
	} else {
		nav := []gotgbot.InlineKeyboardButton{
			{Text: "◀ 上一步", CallbackData: "nav:prev"},
			{Text: "下一步 ▶", CallbackData: "nav:next"},
		}
		if !step.Required {
			nav = append(nav, gotgbot.InlineKeyboardButton{Text: "跳过", CallbackData: "nav:skip"})
		}
		rows = append(rows, nav)
		rows = append(rows, []gotgbot.InlineKeyboardButton{
			{Text: "📋 跳到任意步骤", CallbackData: "nav:menu"},
			{Text: "✅ 提交审核", CallbackData: "submit"},
		})
	}
	return gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func (m *Manager) sendStepMenu(b *gotgbot.Bot, ctx context.Context, d *submission.Draft, current submission.Step) error {
	rows := [][]gotgbot.InlineKeyboardButton{}
	for _, s := range submission.Steps() {
		marker := "·"
		if s.Required {
			marker = "*"
		}
		if v := d.GetString(s.Key); v != "" {
			marker = "✓"
		}
		rows = append(rows, []gotgbot.InlineKeyboardButton{{
			Text:         fmt.Sprintf("%s %s", marker, s.Title),
			CallbackData: "step:" + s.Key,
		}})
	}
	rows = append(rows, []gotgbot.InlineKeyboardButton{
		{Text: "返回", CallbackData: "step:" + current.Key},
	})
	chatID, msgID, _ := m.drafts.LatestMainMessage(ctx, d.ID)
	text := "选择想要修改的步骤：\n* = 必填，✓ = 已填写"
	if chatID != 0 && msgID != 0 {
		_, _, err := b.EditMessageText(text, &gotgbot.EditMessageTextOpts{
			ChatId:      chatID,
			MessageId:   msgID,
			ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows},
		})
		return err
	}
	_, err := b.SendMessage(d.SubmitterChatID, text, &gotgbot.SendMessageOpts{
		ReplyMarkup: gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows},
	})
	return err
}

// --- helpers ---

func requiredMissing(d *submission.Draft) []string {
	var out []string
	for _, s := range submission.Steps() {
		if !s.Required || s.Kind == submission.StepFinal {
			continue
		}
		v := d.GetString(s.Key)
		if v == "" && s.Kind != submission.StepImage {
			out = append(out, s.Title)
			continue
		}
		if s.Kind == submission.StepImage && v == "" {
			// image required: also accept if at least one asset of role exists
			has := false
			for _, a := range d.Assets {
				if a.Role == s.AssetRole {
					has = true
					break
				}
			}
			if !has {
				out = append(out, s.Title)
			}
		}
	}
	return out
}

func (m *Manager) notifyAdminsOfNewDraft(draftID string) {
	bot := m.Bot()
	if bot == nil {
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admins, err := m.authStore.ListAdmins(c)
	if err != nil {
		m.logger.Warn("list admins for notification", "err", err)
		return
	}

	caption := fmt.Sprintf("📨 新投稿待审\n请到管理后台审核 ↓\n%s/admin/review/%s",
		m.SiteURL(), draftID)

	var screenshot []byte
	if m.preview != nil {
		base := m.internalURL
		if base == "" {
			base = m.SiteURL()
		}
		shot, err := m.preview.CaptureDraft(c, base, draftID)
		if err != nil {
			m.logger.Warn("preview capture failed; falling back to text-only", "err", err)
		} else {
			screenshot = shot
		}
	}

	for _, a := range admins {
		if a.TelegramID == 0 {
			continue
		}
		if screenshot != nil {
			photo := gotgbot.InputFileByReader("preview.png", bytesReader(screenshot))
			if _, err := bot.SendPhoto(a.TelegramID, photo, &gotgbot.SendPhotoOpts{Caption: caption}); err == nil {
				continue
			} else {
				m.logger.Warn("send preview photo", "err", err)
			}
		}
		if _, err := bot.SendMessage(a.TelegramID, caption, nil); err != nil {
			m.logger.Warn("notify admin", "tg_id", a.TelegramID, "err", err)
		}
	}
}

func bytesReader(b []byte) interface{ Read(p []byte) (int, error) } {
	return &byteReader{buf: b}
}

type byteReader struct {
	buf []byte
	off int
}

func (r *byteReader) Read(p []byte) (int, error) {
	if r.off >= len(r.buf) {
		return 0, io.EOF
	}
	n := copy(p, r.buf[r.off:])
	r.off += n
	return n, nil
}

// htmlEscape escapes for parse_mode=HTML.
func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

func safeName(u *gotgbot.User) string {
	if u == nil {
		return "朋友"
	}
	if u.FirstName != "" {
		return u.FirstName
	}
	if u.Username != "" {
		return "@" + u.Username
	}
	return "朋友"
}
