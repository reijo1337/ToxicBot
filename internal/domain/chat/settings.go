package chat

import "time"

type ChatSettings struct {
	ThresholdCount     *int
	ThresholdTime      *time.Duration
	Cooldown           *time.Duration
	StickerReactChance *float32
	VoiceReactChance   *float32
	AIChance           *float32
}
