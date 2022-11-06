package utils

import (
	"gopkg.in/telebot.v3"
)

func GetStickersFromPacks(bot *telebot.Bot, stickerPacksNames []string) ([]string, error) {

	stickers := []string{}

	for _, pack := range stickerPacksNames {
		stickerPack, err := bot.StickerSet(pack)
		if err != nil {
			return nil, err
		}

		for _, sticker := range stickerPack.Stickers {
			if sticker.FileID != "" {
				stickers = append(stickers, sticker.FileID)
			}
		}
	}

	return stickers, nil
}
