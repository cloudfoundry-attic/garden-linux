package network

func NewConfigurer() *Configurer {
	return &Configurer{
		Link:   Link{},
		Bridge: Bridge{},
		Veth:   VethCreator{},
	}
}
