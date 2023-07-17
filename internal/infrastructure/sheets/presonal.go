package sheets

type Presonal struct {
	sheets sheets
	name   string
}

func (r *Repository) GetPersonal(name string) *Presonal {
	return &Presonal{
		name:   name,
		sheets: r.sheets,
	}
}

func (p *Presonal) GetEnabledMessages() ([]string, error) {
	return getEditableMessagesFromSheet(p.sheets, p.name)
}
