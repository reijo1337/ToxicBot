package storage

type DTOType interface {
	GreetingsDTO | IgorDTO | RandomDTO | StikersDTO | VoiceDTO | MaxDTO
}

type GreetingsDTO struct {
	Text      string
	IsEnabled bool
}

type GreetingsDTOs []GreetingsDTO

func (a GreetingsDTOs) GetEnabled() GreetingsDTOs {
	temp := a[:0]
	for _, v := range a {
		if v.IsEnabled {
			temp = append(temp, v)
		}
	}
	return temp
}

type IgorDTO struct {
	Text      string
	IsEnabled bool
}

type IgorDTOs []IgorDTO

func (a IgorDTOs) GetEnabled() IgorDTOs {
	temp := a[:0]
	for _, v := range a {
		if v.IsEnabled {
			temp = append(temp, v)
		}
	}
	return temp
}

type MaxDTO struct {
	Text      string
	IsEnabled bool
}

type MaxDTOs []MaxDTO

func (a MaxDTOs) GetEnabled() MaxDTOs {
	temp := a[:0]
	for _, v := range a {
		if v.IsEnabled {
			temp = append(temp, v)
		}
	}
	return temp
}

type RandomDTO struct {
	Text      string
	IsEnabled bool
}

type RandomDTOs []RandomDTO

func (a RandomDTOs) GetEnabled() RandomDTOs {
	temp := a[:0]
	for _, v := range a {
		if v.IsEnabled {
			temp = append(temp, v)
		}
	}
	return temp
}

type StikersDTO struct {
	StickerID string
	IsEnabled bool
}

type StikersDTOs []StikersDTO

func (a StikersDTOs) GetEnabled() StikersDTOs {
	temp := a[:0]
	for _, v := range a {
		if v.IsEnabled {
			temp = append(temp, v)
		}
	}
	return temp
}

type VoiceDTO struct {
	VoiceID   string
	IsEnabled bool
}

type VoiceDTOs []VoiceDTO

func (a VoiceDTOs) GetEnabled() VoiceDTOs {
	temp := a[:0]
	for _, v := range a {
		if v.IsEnabled {
			temp = append(temp, v)
		}
	}
	return temp
}
