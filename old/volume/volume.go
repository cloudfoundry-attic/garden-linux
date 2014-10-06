package volume

type volume struct {
	id     string
	handle string
}

func (volume *volume) ID() string {
	return volume.id
}

func (volume *volume) Handle() string {
	return volume.handle
}
