package actor

type System struct {
	config   *Config
	kinds    map[string]tKind
	registry *Registry
}

func NewSystem(config *Config) *System {
	system := &System{}
	system.config = config
	system.kinds = make(map[string]tKind)
	system.registry = newRegistry(system)
	return system
}

func (x *System) Start() error {
	return nil
}
