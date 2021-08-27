package leave

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"

const text = "Порвался"

func Handler(message *tgbotapi.Message) (tgbotapi.Chattable, error) {
	if message.LeftChatMember == nil {
		return nil, nil
	}

	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyToMessageID = message.MessageID

	return msg, nil
}
