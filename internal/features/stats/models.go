package stats

import (
	"time"

	"github.com/reijo1337/ToxicBot/internal/message"
)

type OperationType uint8

const (
	OnTextOperationType OperationType = iota
	OnStickerOperationType
	OnVoiceOperationType
	OnUserJoinOperationType
	OnUserLeftOperationType
	PersonalOperationType
	TaggerOperationType
)

type Response struct {
	Date          time.Time // date only
	OperationType OperationType
	ChatIDHash    []byte
	UserIDHash    []byte
	Extra         *ResponseExtra
}

type ResponseExtra struct {
	TextGenerationType message.GenerationStrategy
}

type TotalStat struct {
	ByOpTypeStat  map[OperationType]uint64
	ByGenTypeStat map[message.GenerationStrategy]uint64
	BulledChats   uint64
	BulledUsers   uint64
	OldestDate    time.Time
}

type DetailedStat struct {
	ChatNumber    uint64
	BulledUsers   uint64
	ByOpTypeStat  map[OperationType]uint64
	ByGenTypeStat map[message.GenerationStrategy]uint64
}
