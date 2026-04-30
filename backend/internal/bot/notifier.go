package bot

import (
	"context"
	"fmt"

	"github.com/PaulSonOfLars/gotgbot/v2"

	"github.com/ripyc/rip-lgbt-submission-tg/internal/submission"
)

// NotifyUser implements submission.Notifier; it pushes a message to the
// submitter's TG chat. For "revision" it also rewinds the bot conversation
// to the requested step.
func (m *Manager) NotifyUser(ctx context.Context, draft *submission.Draft, kind, message string) error {
	bot := m.Bot()
	if bot == nil {
		return fmt.Errorf("bot offline")
	}
	if draft.SubmitterChatID == 0 {
		return fmt.Errorf("draft has no chat id")
	}
	if _, err := bot.SendMessage(draft.SubmitterChatID, message, nil); err != nil {
		m.logger.Warn("notify user", "err", err)
	}
	if kind == "revision" && draft.CurrentStep != "" {
		step, ok := submission.FindStep(draft.CurrentStep)
		if ok {
			return m.sendStepMessage(bot, ctx, draft, step, true)
		}
	}
	return nil
}

// NotifyAdmins broadcasts to all admins with a TG id.
func (m *Manager) NotifyAdmins(ctx context.Context, draft *submission.Draft, kind, message string) error {
	bot := m.Bot()
	if bot == nil {
		return nil
	}
	admins, err := m.authStore.ListAdmins(ctx)
	if err != nil {
		return err
	}
	for _, a := range admins {
		if a.TelegramID == 0 {
			continue
		}
		if _, err := bot.SendMessage(a.TelegramID, message, nil); err != nil {
			m.logger.Warn("notify admin", "tg_id", a.TelegramID, "err", err)
		}
	}
	_ = draft
	return nil
}

// for compile
var _ = (gotgbot.Bot{})
