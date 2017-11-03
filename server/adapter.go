package server

import (
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/oklahomer/go-sarah"
	"github.com/oklahomer/go-sarah/log"
	"golang.org/x/net/context"
	"net/http"
)

const (
	// IoTServer is a designated sara.BotType for iot server.
	IoTServer sarah.BotType = "iot-server"
)

// AdapterOption defines function signature that Adapter's functional option must satisfy.
type AdapterOption func(*adapter) error

// WithUpgrader creates AdapterOption with given *websocket.Upgrader.
func WithUpgrader(upgrader *websocket.Upgrader) AdapterOption {
	return func(a *adapter) error {
		a.upgrader = upgrader
		return nil
	}
}

// Config contains some configuration variables for iot server Adapter.
type Config struct {
	WebSocketPath string `json:"websocket_path" yaml:"websocket_path"`
	ListenPort    int    `json:"port" yaml:"port"`
}

// NewConfig returns pointer to Config struct with default settings.
// To override each field values, pass this instance to jsono.Unmarshal/yaml.Unmarshal or assign directly.
func NewConfig() *Config {
	return &Config{
		WebSocketPath: "/device",
		ListenPort:    80,
	}
}

type adapter struct {
	config     *Config
	authorizer Authorizer
	upgrader   *websocket.Upgrader
}

var _ sarah.Adapter = (*adapter)(nil)

// BotType returns sarah.BotType of this particular instance.
func (a *adapter) BotType() sarah.BotType {
	return IoTServer
}

// Run initiates its internal server and starts receiving/establishing connections from enslaved devices.
func (a *adapter) Run(_ context.Context, _ func(sarah.Input) error, errNotifier func(error)) {
	c := make(chan connection)

	// TODO Provide a goroutine that receives established connection from above channel and supervise.

	// TODO provide WithServerMux to accept external mux
	// so developer may re-use her mux instance and share same server port.
	mux := http.NewServeMux()
	mux.HandleFunc(a.config.WebSocketPath, serverHandleFunc(a.authorizer, a.upgrader, c))
	err := http.ListenAndServe(fmt.Sprintf(":%d", a.config.ListenPort), mux)
	errNotifier(err) // http.ListenAndServe always returns non-nil error
}

// SendMessage let Bot send message to enslaved devices.
func (a *adapter) SendMessage(context.Context, sarah.Output) {
	panic("implement me")
}

// NewAdapter creates new Adapter with given *Config, Authorizer implementation and arbitrary amount of AdapterOption instances.
func NewAdapter(c *Config, authorizer Authorizer, options ...AdapterOption) (sarah.Adapter, error) {
	a := &adapter{
		config:     c,
		authorizer: authorizer,
	}

	for _, opt := range options {
		err := opt(a)
		if err != nil {
			return nil, fmt.Errorf("failed to execute AdapterOption: %s", err.Error())
		}
	}

	// If no *websocket.Upgrader is provided via AdapterOption, set default upgrader.
	if a.upgrader == nil {
		a.upgrader = &websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		}
	}

	return a, nil
}

func serverHandleFunc(authorizer Authorizer, upgrader *websocket.Upgrader, c chan<- connection) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, req *http.Request) {
		device, err := authorizer.Authorize(req)
		if err != nil {
			// TODO switch error depending on error type
			http.Error(writer, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}

		conn, err := upgrader.Upgrade(writer, req, nil)
		if err != nil {
			log.Warnf("Failed to upgrade protocol: %s.", err.Error())
			return
		}

		c <- &connWrapper{
			conn:   conn,
			device: device,
		}
	}
}

type connection interface {
	// TODO define interface
}

type connWrapper struct {
	conn   *websocket.Conn
	device *Device
}

var _ connection = (*connWrapper)(nil)
