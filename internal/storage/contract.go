package storage

type Manager interface {
	GetGreetings() (greetings GreetingsDTOs, err error)
	GetIgors() (igors IgorDTOs, err error)
	GetMaxs() (maxs MaxDTOs, err error)
	GetRandom() (randoms RandomDTOs, err error)
	GetStickers() (stickers StikersDTOs, err error)
	GetVoices() (stickers VoiceDTOs, err error)
}
