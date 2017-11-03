package server

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/oklahomer/go-sarah"
	"github.com/oklahomer/go-sarah-iot/payload"
	"github.com/oklahomer/go-sarah/log"
	"golang.org/x/net/context"
)

const (
	// IoTServer is a designated sara.BotType for iot server.
	IoTServer sarah.BotType = "iot-server"
)

// AdapterOption defines function signature that Adapter's functional option must satisfy.
type AdapterOption func(*adapter) error

// WithDecoder creates AdapterOption with given payload.Decoder implementation.
func WithDecoder(decoder payload.Decoder) AdapterOption {
	return func(a *adapter) error {
		a.decoder = decoder
		return nil
	}
}

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
	decoder    payload.Decoder
	upgrader   *websocket.Upgrader
}

var _ sarah.Adapter = (*adapter)(nil)

// BotType returns sarah.BotType of this particular instance.
func (a *adapter) BotType() sarah.BotType {
	return IoTServer
}

// Run initiates its internal server and starts receiving/establishing connections from enslaved devices.
func (a *adapter) Run(ctx context.Context, input func(sarah.Input) error, errNotifier func(error)) {
	c := make(chan connection)

	go a.superviseConnection(ctx, c, input)

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

func (a *adapter) superviseConnection(ctx context.Context, incomingConn <-chan connection, input func(input sarah.Input) error) {
	var conns []connection
	closingConn := make(chan connection, 1)

	inputSwitcher := func(i sarah.Input) error {
		switch i.(type) {
		case *Input:
			return input(i)

		case *ResponseInput:
			// TODO handle response with stored transaction data
			return nil

		default:
			return fmt.Errorf("uncontrollable input type is given: %T", i)

		}
	}

	for {
		select {
		case <-ctx.Done():
			log.Info("Closing connections due to context cancellation.")
			for _, conn := range conns {
				err := conn.Close()
				if err != nil {
					log.Warnf("Error while closing connection: %s.", err.Error())
				}
			}
			log.Info("Finished closing all connections.")

			return

		case conn := <-incomingConn:
			go func() {
				err := a.receivePayload(conn, inputSwitcher)
				if err != nil {
					// Connection is stale or is intentionally closed.
					log.Errorf(
						"Failed to read from stale connection: %s. ID: %s.",
						err.Error(),
						conn.Device().ID,
					)
				}

				// Make sure to close at the end.
				closingConn <- conn
			}()

			conns = append(conns, conn)

		case c := <-closingConn:
			connFound := false
			for _, conn := range conns {
				if conn != c {
					continue
				}
				connFound = true

				log.Infof("Closing connection: %s.", conn.Device().ID)
				err := conn.Close()
				if err != nil {
					log.Warnf("Error while closing connection: %s.", err.Error())
				}

				break
			}

			if !connFound {
				log.Infof("Tried to close connection, but was not found. Probably closed on recent context cancellation.")
			}
		}
	}
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

	// If no payload.Decoder is provided via AdapterOption, set default decoder.
	if a.decoder == nil {
		a.decoder = payload.NewDecoder()
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

type Input struct {
	Content   interface{}
	Device    *Device
	TimeStamp time.Time
}

var _ sarah.Input = (*Input)(nil)

func (i Input) SenderKey() string {
	return i.Device.ID
}

func (i *Input) Message() string {
	return ""
}

func (i *Input) SentAt() time.Time {
	return i.TimeStamp
}

func (i *Input) ReplyTo() sarah.OutputDestination {
	return i.Device
}

type ResponseInput struct {
	TransactionID string
	Content       interface{}
	Device        *Device
	TimeStamp     time.Time
}

var _ sarah.Input = (*ResponseInput)(nil)

func (i ResponseInput) SenderKey() string {
	return i.Device.ID
}

func (i *ResponseInput) Message() string {
	return ""
}

func (i *ResponseInput) SentAt() time.Time {
	return i.TimeStamp
}

func (i *ResponseInput) ReplyTo() sarah.OutputDestination {
	return i.Device
}

func (a *adapter) receivePayload(conn connection, input func(input sarah.Input) error) error {
	device := conn.Device()

	for {
		// "Once this method returns a non-nil error, all subsequent calls to this method return the same error."
		messageType, reader, err := conn.NextReader()
		if err != nil {
			return fmt.Errorf("failed to read incoming message: %s", err.Error())
		}

		switch messageType {
		case websocket.TextMessage, websocket.BinaryMessage:
			p, err := a.decoder.Decode(messageType, reader)
			if err != nil {
				log.Errorf("Failed to decode payload: %s.", err.Error())
				continue
			}

			switch typed := p.(type) {
			case *payload.RoledPayload:
				// IoT device may not have access to accurate time.
				// If not, fill it with current server time.
				timeStamper, ok := typed.Content.(payload.TimeStamper)
				var timestamp time.Time
				if ok {
					timestamp = timeStamper.SentAt()
				} else {
					timestamp = time.Now()
				}

				i := &Input{
					Content:   typed.Content,
					Device:    device,
					TimeStamp: timestamp,
				}
				input(i)

			case *payload.ResponsePayload:
				// IoT device may not have access to accurate time.
				// If not, fill it with current server time.
				timeStamper, ok := typed.Content.(payload.TimeStamper)
				var timestamp time.Time
				if ok {
					timestamp = timeStamper.SentAt()
				} else {
					timestamp = time.Now()
				}

				r := &ResponseInput{
					TransactionID: typed.TransactionID,
					Content:       typed.Content,
					Device:        device,
					TimeStamp:     timestamp,
				}
				input(r)

			default:
				log.Errorf("Uncontrollable payload type is returned: %+v", p)
				continue

			}

		case websocket.CloseMessage:
			return nil

		case websocket.PingMessage:
			err := conn.Pong()
			if err != nil {
				return fmt.Errorf("failed to send Pong message: %s", err.Error())
			}

		case websocket.PongMessage:
			// O.K.

		default:
			log.Errorf("Unknown message type is returned: %d.", messageType)

		}
	}
}

type connection interface {
	Close() error
	NextReader() (int, io.Reader, error)
	Pong() error
	Device() *Device
}

type connWrapper struct {
	conn   *websocket.Conn
	device *Device
}

func (cw *connWrapper) Close() error {
	return cw.conn.Close()
}

func (cw *connWrapper) NextReader() (int, io.Reader, error) {
	return cw.conn.NextReader()
}

func (cw *connWrapper) Pong() error {
	return cw.conn.WriteMessage(websocket.PongMessage, []byte{})
}

func (cw *connWrapper) Device() *Device {
	return cw.device
}

var _ connection = (*connWrapper)(nil)
