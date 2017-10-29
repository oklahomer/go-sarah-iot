package server

import (
	"fmt"
	"github.com/oklahomer/go-sarah"
	"golang.org/x/net/context"
)

const (
	// IoTServer is a designated sara.BotType for iot server.
	IoTServer sarah.BotType = "iot-server"
)

// AdapterOption defines function signature that Adapter's functional option must satisfy.
type AdapterOption func(*adapter) error

// Config contains some configuration variables for iot server Adapter.
type Config struct {
}

// NewConfig returns pointer to Config struct with default settings.
// To override each field values, pass this instance to jsono.Unmarshal/yaml.Unmarshal or assign directly.
func NewConfig() *Config {
	return &Config{}
}

type adapter struct {
	config *Config
}

var _ sarah.Adapter = (*adapter)(nil)

// BotType returns sarah.BotType of this particular instance.
func (a *adapter) BotType() sarah.BotType {
	return IoTServer
}

// Run initiates its internal server and starts receiving/establishing connections from enslaved devices.
func (a *adapter) Run(context.Context, func(sarah.Input) error, func(error)) {
	panic("implement me")
}

// SendMessage let Bot send message to enslaved devices.
func (a *adapter) SendMessage(context.Context, sarah.Output) {
	panic("implement me")
}

// NewAdapter creates new Adapter with given *Config and arbitrary amount of AdapterOption instances.
func NewAdapter(c *Config, options ...AdapterOption) (sarah.Adapter, error) {
	a := &adapter{
		config: c,
	}

	for _, opt := range options {
		err := opt(a)
		if err != nil {
			return nil, fmt.Errorf("failed to execute AdapterOption: %s", err.Error())
		}
	}

	return a, nil
}
