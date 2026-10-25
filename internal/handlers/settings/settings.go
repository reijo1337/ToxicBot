package settings

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/reijo1337/ToxicBot/internal/domain/chat"
	"github.com/reijo1337/ToxicBot/internal/features/chatsettings"
	"gopkg.in/telebot.v3"
)

type settingsProvider interface {
	GetForChat(ctx context.Context, chatID int64) (*chatsettings.Settings, error)
	UpsertForChat(ctx context.Context, chatID int64, partial chat.ChatSettings) error
	ResetChat(ctx context.Context, chatID int64) error
}

type Handler struct {
	provider settingsProvider
	defaults chatsettings.Defaults
}

func New(provider settingsProvider, defaults chatsettings.Defaults) *Handler {
	return &Handler{
		provider: provider,
		defaults: defaults,
	}
}

func (h *Handler) Handle(ctx telebot.Context) error {
	message := ctx.Message()
	tgChat := ctx.Chat()

	if message == nil || tgChat == nil {
		return nil
	}

	allowed, err := canManageSettings(ctx, tgChat, message)
	if err != nil || !allowed {
		return err
	}

	// Разбираем команду: /settings [key [value]]
	text := message.Text
	parts := strings.Fields(text)
	// parts[0] == "/settings"

	switch len(parts) {
	case 1:
		return h.handleShow(ctx, tgChat.ID)
	case 2:
		if parts[1] == "reset" {
			return h.handleReset(ctx, tgChat.ID)
		}
		return ctx.Reply(h.usage())
	case 3:
		return h.handleSet(ctx, tgChat.ID, parts[1], parts[2])
	default:
		return ctx.Reply(h.usage())
	}
}

func canManageSettings(
	ctx telebot.Context,
	tgChat *telebot.Chat,
	message *telebot.Message,
) (bool, error) {
	if tgChat.Type == telebot.ChatPrivate {
		return false, ctx.Reply("Иди в группу, тут тебе не личный кабинет психотерапевта.")
	}
	if tgChat.Type != telebot.ChatGroup && tgChat.Type != telebot.ChatSuperGroup {
		return false, ctx.Reply("Команда доступна только в группе.")
	}

	if message.SenderChat != nil {
		if tgChat.Type == telebot.ChatSuperGroup && message.SenderChat.ID == tgChat.ID {
			return true, nil
		}
		return false, ctx.Reply(
			"Вызови команду как администратор от своего аккаунта или от имени этой группы.",
		)
	}

	sender := ctx.Sender()
	if sender == nil {
		return false, nil
	}

	member, err := ctx.Bot().ChatMemberOf(tgChat, sender)
	if err != nil {
		return false, verificationFailed(ctx, err)
	}
	if member == nil {
		return false, verificationFailed(ctx, errors.New("empty chat member"))
	}

	switch member.Role {
	case telebot.Creator, telebot.Administrator:
		return true, nil
	case telebot.Member, telebot.Restricted, telebot.Left, telebot.Kicked:
		return false, ctx.Reply("Настройки чата доступны только администраторам.")
	default:
		return false, verificationFailed(
			ctx,
			fmt.Errorf("unknown chat member role %q", member.Role),
		)
	}
}

// verificationFailed отказывает в команде и возвращает причину, чтобы она попала в лог.
func verificationFailed(ctx telebot.Context, cause error) error {
	return errors.Join(
		fmt.Errorf("check chat administrator: %w", cause),
		ctx.Reply("Не удалось проверить права. Попробуй ещё раз позже."),
	)
}

func (h *Handler) handleShow(ctx telebot.Context, chatID int64) error {
	s, err := h.provider.GetForChat(context.Background(), chatID)
	if err != nil {
		return fmt.Errorf("can't get settings: %w", err)
	}

	msg := fmt.Sprintf(
		"⚙️ Настройки чата:\n"+
			"• threshold_count: %d (по умолчанию: %d)\n"+
			"• threshold_time: %s (по умолчанию: %s)\n"+
			"• cooldown: %s (по умолчанию: %s)\n"+
			"• sticker_chance: %.2f (по умолчанию: %.2f)\n"+
			"• voice_chance: %.2f (по умолчанию: %.2f)\n"+
			"• ai_chance: %.2f (по умолчанию: %.2f)\n"+
			"• photo_chance: %.2f (по умолчанию: %.2f)\n"+
			"• tagger: %s (по умолчанию: %s)\n\n"+
			"Изменить: /settings <ключ> <значение>\n"+
			"Сбросить: /settings reset",
		s.ThresholdCount, h.defaults.ThresholdCount,
		s.ThresholdTime, h.defaults.ThresholdTime,
		s.Cooldown, h.defaults.Cooldown,
		s.StickerReactChance, h.defaults.StickerReactChance,
		s.VoiceReactChance, h.defaults.VoiceReactChance,
		s.AIChance, h.defaults.AIChance,
		s.PhotoReactChance, h.defaults.PhotoReactChance,
		onOff(s.TaggerEnabled), onOff(h.defaults.TaggerEnabled),
	)

	return ctx.Reply(msg)
}

func (h *Handler) handleReset(ctx telebot.Context, chatID int64) error {
	if err := h.provider.ResetChat(context.Background(), chatID); err != nil {
		return fmt.Errorf("can't reset settings: %w", err)
	}

	return ctx.Reply("Ладно, сброшено. Можешь радоваться, всё как было — убого и по дефолту.")
}

func (h *Handler) handleSet(ctx telebot.Context, chatID int64, key, rawValue string) error {
	var partial chat.ChatSettings
	var errMsg string

	switch key {
	case "threshold_count":
		v, err := strconv.Atoi(rawValue)
		if err != nil || v <= 0 {
			errMsg = "Ты числа-то видел вообще? threshold_count — это положительное целое, а не вот эта хрень. Пример: 5"
		} else {
			partial.ThresholdCount = &v
		}

	case "threshold_time":
		v, err := time.ParseDuration(rawValue)
		if err != nil || v <= 0 {
			errMsg = "Ну и что это за каракули? threshold_time — это длительность, дурилка. Пиши нормально: 1m, 30s, 2h"
		} else {
			partial.ThresholdTime = &v
		}

	case "cooldown":
		v, err := time.ParseDuration(rawValue)
		if err != nil || v <= 0 {
			errMsg = "Руки из жопы? cooldown — это длительность. Примеры для особо одарённых: 1h, 30m, 15s"
		} else {
			partial.Cooldown = &v
		}

	case "sticker_chance":
		if f, ok := parseChance(rawValue); ok {
			partial.StickerReactChance = &f
		} else {
			errMsg = "Вероятность — это число от 0 до 1, гений. Не \"" + rawValue + "\". Попробуй ещё раз, может в этот раз получится. Пример: 0.4"
		}

	case "voice_chance":
		if f, ok := parseChance(rawValue); ok {
			partial.VoiceReactChance = &f
		} else {
			errMsg = "Число от 0.0 до 1.0, обезьяна. \"" + rawValue + "\" — это не число, это позор. Пример: 0.8"
		}

	case "ai_chance":
		if f, ok := parseChance(rawValue); ok {
			partial.AIChance = &f
		} else {
			errMsg = "Даже ИИ за тебя стыдно. Введи число от 0.0 до 1.0, а не \"" + rawValue + "\". Пример: 0.75"
		}

	case "photo_chance":
		if f, ok := parseChance(rawValue); ok {
			partial.PhotoReactChance = &f
		} else {
			errMsg = "Фоточки тоже считать не умеешь? Число от 0.0 до 1.0, давай заново. Пример: 0.5"
		}

	case "tagger":
		if v, ok := parseOnOff(rawValue); ok {
			partial.TaggerEnabled = &v
		} else {
			errMsg = "Тут всего два варианта, и ты умудрился промахнуться. on или off. Пример: /settings tagger off"
		}

	default:
		return ctx.Reply("Такой настройки не существует, балбес.\n\n" + h.usage())
	}

	if errMsg != "" {
		return ctx.Reply(errMsg)
	}

	if err := h.provider.UpsertForChat(context.Background(), chatID, partial); err != nil {
		return fmt.Errorf("can't update settings: %w", err)
	}

	return ctx.Reply(
		fmt.Sprintf("О, смотрите-ка, справился. %s = %s. Мамка гордилась бы.", key, rawValue),
	)
}

func parseChance(raw string) (float32, bool) {
	v, err := strconv.ParseFloat(raw, 32)
	if err != nil || v < 0 || v > 1 {
		return 0, false
	}
	return float32(v), true
}

func parseOnOff(raw string) (bool, bool) {
	switch raw {
	case "on":
		return true, true
	case "off":
		return false, true
	default:
		return false, false
	}
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func (h *Handler) usage() string {
	return "Использование:\n" +
		"/settings — показать настройки\n" +
		"/settings reset — сбросить настройки\n" +
		"/settings <ключ> <значение> — установить настройку\n\n" +
		"Доступные ключи:\n" +
		"• threshold_count (целое число, > 0)\n" +
		"• threshold_time (длительность: 30s, 1m, 2h)\n" +
		"• cooldown (длительность: 30m, 1h)\n" +
		"• sticker_chance (0.0 – 1.0)\n" +
		"• voice_chance (0.0 – 1.0)\n" +
		"• ai_chance (0.0 – 1.0)\n" +
		"• photo_chance (0.0 – 1.0)\n" +
		"• tagger (on / off)"
}
