package settings

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/reijo1337/ToxicBot/internal/chatsettings"
	"github.com/reijo1337/ToxicBot/internal/domain/chat"
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
	tgChat := ctx.Chat()
	sender := ctx.Sender()

	if tgChat == nil || sender == nil {
		return nil
	}

	if tgChat.Type == telebot.ChatPrivate {
		return ctx.Reply("Иди в группу, тут тебе не личный кабинет психотерапевта.")
	}

	// Проверка прав администратора
	member, err := ctx.Bot().ChatMemberOf(tgChat, sender)
	if err != nil {
		return fmt.Errorf("can't get chat member: %w", err)
	}

	if member.Role != telebot.Creator && member.Role != telebot.Administrator {
		return ctx.Reply("Пососи, потом проси.")
	}

	// Разбираем команду: /settings [key [value]]
	text := ctx.Message().Text
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
			"• ai_chance: %.2f (по умолчанию: %.2f)\n\n"+
			"Изменить: /settings <ключ> <значение>\n"+
			"Сбросить: /settings reset",
		s.ThresholdCount, h.defaults.ThresholdCount,
		s.ThresholdTime, h.defaults.ThresholdTime,
		s.Cooldown, h.defaults.Cooldown,
		s.StickerReactChance, h.defaults.StickerReactChance,
		s.VoiceReactChance, h.defaults.VoiceReactChance,
		s.AIChance, h.defaults.AIChance,
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
		v, err := strconv.ParseFloat(rawValue, 32)
		if err != nil || v < 0 || v > 1 {
			errMsg = "Вероятность — это число от 0 до 1, гений. Не \"" + rawValue + "\". Попробуй ещё раз, может в этот раз получится"
		} else {
			f := float32(v)
			partial.StickerReactChance = &f
		}

	case "voice_chance":
		v, err := strconv.ParseFloat(rawValue, 32)
		if err != nil || v < 0 || v > 1 {
			errMsg = "Число от 0.0 до 1.0, обезьяна. \"" + rawValue + "\" — это не число, это позор"
		} else {
			f := float32(v)
			partial.VoiceReactChance = &f
		}

	case "ai_chance":
		v, err := strconv.ParseFloat(rawValue, 32)
		if err != nil || v < 0 || v > 1 {
			errMsg = "Даже ИИ за тебя стыдно. Введи число от 0.0 до 1.0, а не \"" + rawValue + "\""
		} else {
			f := float32(v)
			partial.AIChance = &f
		}

	default:
		return ctx.Reply(
			"Такой настройки не существует, балбес. Попробуй /settings без параметров, если забыл как пользоваться.",
		)
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
		"• ai_chance (0.0 – 1.0)"
}
