package actor

type System struct {
	config   *Config
	registry *Registry
}

func NewSystem(config *Config) *System {
	system := &System{}
	system.config = config
	system.registry = newRegistry(system)
	return system
}

func (x *System) Start() error {
	return nil
}

func (x *System) Stop() error {
	return nil
}

func (x *System) GetConfig() *Config {
	return x.config
}

func (x *System) GetRegistry() *Registry {
	return x.registry
}
