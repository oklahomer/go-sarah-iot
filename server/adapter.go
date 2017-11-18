package server

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/oklahomer/go-sarah"
	"github.com/oklahomer/go-sarah-iot/auth"
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

// WithEncoder creates AdapterOption with given payload.Encoder implementation.
func WithEncoder(encoder payload.Encoder) AdapterOption {
	return func(a *adapter) error {
		a.encoder = encoder
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

// WithTransactionStorage creates AdapterOption with given TransactionStorage.
func WithTransactionStorage(storage TransactionStorage) AdapterOption {
	return func(a *adapter) error {
		a.storage = storage
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
	authorizer auth.Authorizer
	decoder    payload.Decoder
	encoder    payload.Encoder
	upgrader   *websocket.Upgrader
	storage    TransactionStorage
	output     chan sarah.Output
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
func (a *adapter) SendMessage(ctx context.Context, o sarah.Output) {
	// Loosely validate type before enqueue.
	switch o.(type) {
	case *output, *transactionalOutput:
		a.output <- o

	default:
		log.Errorf("Uncontrollable output type is passed: %T", o)

	}
}

func (a *adapter) superviseConnection(ctx context.Context, incomingConn <-chan connection, input func(input sarah.Input) error) {
	var conns []connection
	closingConn := make(chan connection, 1)

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
				callback := func(i sarah.Input) error {
					return handleInput(i, a.storage, input)
				}
				err := a.receivePayload(conn, callback)
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

		case o := <-a.output:
			messageType, outgoing, err := encodeOutput(o, a.encoder)
			if err != nil {
				log.Errorf("Failed to encode output: %+v", err.Error())
				continue
			}

			i := 0
			for _, conn := range extractDestConns(o.Destination(), conns) {
				err := conn.WriteMessage(messageType, outgoing)
				if err != nil {
					// TODO enqueue ping at this point?
					log.Errorf("Failed to send message: %s. %s.", conn.Device().ID, err.Error())
					continue
				}

				i++
			}

			t, isTransactional := o.(*transactionalOutput)
			if isTransactional {
				response := make(chan *Response, i)
				a.storage.Add(t.transactionID, response)
				go awaitResponse(ctx, i, response, t.c)
			}

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

func handleInput(input sarah.Input, storage TransactionStorage, receiveInput func(sarah.Input) error) error {
	switch typed := input.(type) {
	case *Input:
		// Pass it to sarah.Runner to search and execute corresponding sarah.Command.
		return receiveInput(input)

	case *ResponseInput:
		// Incoming payload has transaction ID with it.
		// Consider this as response from enslaved devices.
		ch, err := storage.Get(typed.TransactionID)
		if err != nil {
			return err
		}

		select {
		case ch <- &Response{Content: typed.Content, Device: typed.Device}:
			// O.K.
			return nil

		default:
			// Receiving channel should have large enough buffer to ensure enqueue operation does not block.
			// Ideal buffer length is equal to the number of expecting responses,
			// which is the number of Devices request is being sent.
			return fmt.Errorf(
				"failed to pass response from device %s due to lack of channel capacity",
				typed.Device.ID,
			)

		}

	default:
		return fmt.Errorf("uncontrollable input type is given: %T", input)

	}
}

func encodeOutput(o sarah.Output, e payload.Encoder) (int, []byte, error) {
	switch typed := o.(type) {
	case *output:
		return e.EncodeRoled(typed.destination.role, typed.content)

	case *transactionalOutput:
		return e.EncodeTransactional(typed.transactionID, typed.destination.role, typed.content)

	default:
		return 0, nil, fmt.Errorf("uncontrollable type of sarah.Output is given: %+v", o)

	}
}

func extractDestConns(destination sarah.OutputDestination, connections []connection) []connection {
	var conns []connection

	d, ok := destination.(*Destination)
	if !(ok) {
		return conns
	}

	// deviceID or iot.Role is always set. See DestinationBuilder.Build.
	if d.deviceID == "" {
		for _, conn := range connections {
			if conn.Device().Roles.Contains(d.role) {
				conns = append(conns, conn)
			}
		}
	} else {
		for _, conn := range connections {
			if d.deviceID == conn.Device().ID {
				// Device ID is unique.
				// If one corresponding connection is found, it's O.K. to skip the rest of the connections.
				conns = append(conns, conn)
				break
			}
		}
	}

	return conns
}

func awaitResponse(ctx context.Context, i int, in <-chan *Response, out chan<- []*Response) {
	var responses []*Response

	notifyResponses := func(o chan<- []*Response, r []*Response) {
		select {
		case o <- r:
			// O.K.

		default:
			log.Errorf("Response can not be sent because the caller is not listening to this channel.")

		}
	}

	if i == 0 {
		notifyResponses(out, responses)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return

		case r := <-in:
			responses = append(responses, r)
			if len(responses) == i {
				notifyResponses(out, responses)
				return
			}

		case <-time.NewTimer(1 * time.Minute).C:
			// Timeout. Send already returned responses and quit.
			notifyResponses(out, responses)
			return

		}
	}
}

// NewAdapter creates new Adapter with given *Config, Authorizer implementation and arbitrary amount of AdapterOption instances.
func NewAdapter(c *Config, authorizer auth.Authorizer, options ...AdapterOption) (sarah.Adapter, error) {
	a := &adapter{
		config:     c,
		authorizer: authorizer,
		output:     make(chan sarah.Output, 100),
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

	// If no payload.Encoder is provided via AdapterOption, set default encoder.
	if a.encoder == nil {
		a.encoder = payload.NewEncoder()
	}

	// If no TransactionStorage is provided via AdapterOption, set default storage.
	if a.storage == nil {
		a.storage = NewTransactionStorage(NewTransactionStorageConfig())
	}

	return a, nil
}

func serverHandleFunc(authorizer auth.Authorizer, upgrader *websocket.Upgrader, c chan<- connection) func(http.ResponseWriter, *http.Request) {
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

// Input represents an input from iot.device.
type Input struct {
	Content   interface{}
	Device    *auth.Device
	TimeStamp time.Time
}

var _ sarah.Input = (*Input)(nil)

// SenderKey returns string representing message sender.
func (i Input) SenderKey() string {
	return i.Device.ID
}

// Message returns sent message.
func (i *Input) Message() string {
	return ""
}

// SentAt returns message event's timestamp.
func (i *Input) SentAt() time.Time {
	return i.TimeStamp
}

// ReplyTo returns replying destination.
func (i *Input) ReplyTo() sarah.OutputDestination {
	return i.Device
}

// ResponseInput represents an response from iot.device.
type ResponseInput struct {
	TransactionID string
	Content       interface{}
	Device        *auth.Device
	TimeStamp     time.Time
}

var _ sarah.Input = (*ResponseInput)(nil)

// SenderKey returns string representing message sender.
func (i ResponseInput) SenderKey() string {
	return i.Device.ID
}

// Message returns sent message.
func (i *ResponseInput) Message() string {
	return ""
}

// SentAt returns message event's timestamp.
func (i *ResponseInput) SentAt() time.Time {
	return i.TimeStamp
}

// ReplyTo returns replying destination.
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

		// Do not handle websocket.CloseMessage, websocket.PingMessage or websocket.PongMessage.
		// Those control messages have corresponding handler methods.
		// Those methods can be set via setter method such as Conn.SetPingHandler.
		//
		// When connection is closed, Conn.NextReader returns websocket.CloseError.
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
				timeStamper, ok := typed.Content.(payload.Timestamper)
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
				err := input(i)
				if err != nil {
					log.Errorf("Failed to enqueue incoming RoledPayload: %s.", err.Error())
				}

			case *payload.TransactionalPayload:
				// IoT device may not have access to accurate time.
				// If not, fill it with current server time.
				timeStamper, ok := typed.Content.(payload.Timestamper)
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
				err := input(r)
				if err != nil {
					log.Errorf("Failed to enqueue incoming TransactionalPayload: %s.", err.Error())
				}

			default:
				log.Errorf("Uncontrollable payload type is returned: %+v", p)
				continue

			}

		default:
			log.Errorf("Unknown message type is returned: %d.", messageType)

		}
	}
}

type connection interface {
	Close() error
	NextReader() (int, io.Reader, error)
	WriteMessage(int, []byte) error
	Pong() error
	Device() *auth.Device
}

type connWrapper struct {
	conn   *websocket.Conn
	device *auth.Device
}

var _ connection = (*connWrapper)(nil)

func (cw *connWrapper) Close() error {
	return cw.conn.Close()
}

func (cw *connWrapper) NextReader() (int, io.Reader, error) {
	return cw.conn.NextReader()
}

func (cw *connWrapper) WriteMessage(messageType int, payload []byte) error {
	return cw.conn.WriteMessage(messageType, payload)
}

func (cw *connWrapper) Pong() error {
	return cw.conn.WriteMessage(websocket.PongMessage, []byte{})
}

func (cw *connWrapper) Device() *auth.Device {
	return cw.device
}

var _ connection = (*connWrapper)(nil)
