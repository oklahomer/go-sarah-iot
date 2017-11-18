package device

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/oklahomer/go-sarah"
	"github.com/oklahomer/go-sarah-iot"
	"github.com/oklahomer/go-sarah-iot/payload"
	"github.com/oklahomer/go-sarah/log"
	"github.com/oklahomer/go-sarah/retry"
	"golang.org/x/net/context"
)

var (
	// ConnectedOutputDestination can be used as sarah.OutputDestination instead of assigning specific value each time.
	//
	// In this Adapter, outgoing messages are only sent to central server which is connected via WebSocket connection.
	// This messaging typically does not need any additional information such as "talk room" or "recipient user"
	// so assigning sarah.OutputDestination does not really make sense on regular use case.
	//
	// However sarah.Runner obligates sarah.ScheduledTask.Execute() to return non-nil sarah.OutputDestination
	// or obligates sarah.ScheduledTask.DefaultDestination to be non-nil.
	ConnectedOutputDestination = struct{}{}
)

const (
	// IoTDevice is a designated sarah.BotType for iot device.
	IoTDevice sarah.BotType = "iot-device"
)

// Config contains some configuration variables for iot device Adapter.
type Config struct {
	ServerURL        string `json:"server_url" yaml:"server_url"`
	SendingQueueSize uint   `json:"sending_queue_size" yaml:"sending_queue_size"`
}

// NewConfig returns pointer to Config struct with default settings.
// To override each field values, pass this instance to jsono.Unmarshal/yaml.Unmarshal or assign directly.
func NewConfig() *Config {
	return &Config{
		ServerURL:        "",
		SendingQueueSize: 100,
	}
}

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

type adapter struct {
	config  *Config
	decoder payload.Decoder
	encoder payload.Encoder
	output  chan *outputContent
}

var _ sarah.Adapter = (*adapter)(nil)

// BotType returns sarah.BotType of this particular instance.
func (a *adapter) BotType() sarah.BotType {
	return IoTDevice
}

// Run establishes connection with configured server and starts interaction.
func (a *adapter) Run(ctx context.Context, enqueueInput func(sarah.Input) error, notifyErr func(error)) {
	for {
		conn, err := a.connect()
		if err != nil {
			notifyErr(sarah.NewBotNonContinuableError(err.Error()))
			return
		}

		connErr := a.superviseConnection(ctx, conn, enqueueInput)
		conn.Close() // Make sure to close when connection is no longer supervised.
		if connErr == nil {
			// Connection is intentionally closed by caller with context cancellation.
			// No more interaction follows.
			return
		}

		log.Errorf("Will try re-connection due to previous connection's fatal state: %s.", connErr.Error())
	}
}

// SendMessage let Bot send message to connected server.
func (a *adapter) SendMessage(ctx context.Context, o sarah.Output) {
	// Loosely validate type before enqueue.
	switch typed := o.Content().(type) {
	case *outputContent:
		a.output <- typed

	default:
		log.Errorf("Uncontrollable output type is passed: %T", typed)

	}
}

// NewAdapter creates new Adapter with given *Config and arbitrary amount of AdapterOption instances.
func NewAdapter(config *Config, options ...AdapterOption) (sarah.Adapter, error) {
	a := &adapter{
		config: config,
		output: make(chan *outputContent, config.SendingQueueSize),
	}

	for _, opt := range options {
		err := opt(a)
		if err != nil {
			return nil, fmt.Errorf("failed to execute AdapterOption: %s", err.Error())
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

	return a, nil
}

func (a *adapter) connect() (connection, error) {
	url := a.config.ServerURL
	if url == "" {
		return nil, errors.New("empty url is set to Config.ServerURL")
	}

	var conn *websocket.Conn
	err := retry.WithInterval(10, func() (e error) {
		// TODO set header values for authorization
		header := http.Header{}

		dialer := websocket.Dialer{}
		conn, _, e = dialer.Dial(url, header)

		return
	}, 3*time.Second)

	if err != nil {
		return nil, err
	}

	conn.SetPongHandler(func(appData string) error {
		log.Debugf("Received Pong: %s", appData)
		return nil
	})

	return &connWrapper{
		conn: conn,
	}, err
}

func (a *adapter) receivePayload(conn connection, enqueueInput func(sarah.Input) error) error {
	for {
		// DO NOT try ping or other alternative method to check connection status.
		// See github.com/gorilla/websocket's document for Conn.NextReader.
		//
		// "Applications must break out of the application's read loop when this method
		// returns a non-nil error value. Errors returned from this method are
		// permanent. Once this method returns a non-nil error, all subsequent calls to
		// this method return the same error."
		messageType, reader, err := conn.NextReader()
		if err != nil {
			if err != nil {
				return fmt.Errorf("failed to read incoming message: %s", err.Error())
			}
		}

		switch messageType {
		case websocket.TextMessage, websocket.BinaryMessage:
			p, err := a.decoder.Decode(messageType, reader)
			if err != nil {
				log.Errorf("Failed to decode received payload: %s", err.Error())
				continue
			}

			switch typed := p.(type) {
			case *payload.RoledPayload:
				enqueueInput(&Input{
					Sender:    a.config.ServerURL,
					Content:   typed.Content,
					TimeStamp: time.Now(), // TODO
				})

			case *payload.TransactionalPayload:
				enqueueInput(&TransactionalInput{
					TransactionID: typed.TransactionID,
					Sender:        a.config.ServerURL,
					Content:       typed.Content,
					TimeStamp:     time.Now(), // TODO
				})

			default:
				log.Errorf("Uncontrollable payload type is returned: %+v", p)
				continue

			}

		default:
			log.Errorf("Unknown message type is returned: %d.", messageType)

		}
	}
}

func (a *adapter) superviseConnection(connCtx context.Context, conn connection, enqueueInput func(sarah.Input) error) error {
	pingTicker := time.NewTicker(30 * time.Second) // TODO set via Config
	defer pingTicker.Stop()

	permanentReadErr := make(chan error, 1)
	go func() {
		err := a.receivePayload(conn, enqueueInput)
		if err != nil {
			permanentReadErr <- err
		}
	}()

	for {
		select {
		case <-connCtx.Done():
			return nil

		case err := <-permanentReadErr:
			return err

		case message := <-a.output:
			messageType, b, err := encodeOutput(message, a.encoder)
			if err != nil {
				log.Errorf("Failed to encode outgoing message: %s. %+v.", message, err.Error())
				continue
			}

			switch messageType {
			case websocket.TextMessage, websocket.BinaryMessage:
				err = conn.WriteMessage(messageType, b)
				if err != nil {
					log.Warnf("Failed to send message: %s.", err.Error())
					// When messaging fails, send PING immediately
					// to make sure ping is sent before following messages are processed.
					err = conn.WriteMessage(websocket.PingMessage, []byte{})
					if err != nil {
						return fmt.Errorf("error on ping: %s", err.Error())
					}
				}

			default:
				log.Errorf("Unsupported message type is given: %d. %+v.", messageType, message)

			}

		case <-pingTicker.C:
			log.Debug("Send ping")
			err := conn.WriteMessage(websocket.PingMessage, []byte{})
			if err != nil {
				return fmt.Errorf("error on ping: %s", err.Error())
			}

		}
	}
}

func encodeOutput(o *outputContent, e payload.Encoder) (int, []byte, error) {
	if o.transactionID == "" {
		return e.EncodeRoled(o.role, o.payloadContent)
	}

	return e.EncodeTransactional(o.transactionID, o.role, o.payloadContent)
}

type connection interface {
	Close() error
	NextReader() (int, io.Reader, error)
	WriteMessage(int, []byte) error
	Pong() error
}

type connWrapper struct {
	conn *websocket.Conn
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

// Input represents an input from iot.server.
type Input struct {
	Sender    string
	Content   interface{}
	TimeStamp time.Time
}

var _ sarah.Input = (*Input)(nil)

// SenderKey returns string representing message sender.
func (i Input) SenderKey() string {
	return i.Sender
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
	return ConnectedOutputDestination
}

// TransactionalInput represents an transactional input from iot.server that contains transaction ID to be part of response.
type TransactionalInput struct {
	TransactionID string
	Sender        string
	Content       interface{}
	TimeStamp     time.Time
}

var _ sarah.Input = (*TransactionalInput)(nil)

// SenderKey returns string representing message sender.
func (i TransactionalInput) SenderKey() string {
	return i.Sender
}

// Message returns sent message.
func (i *TransactionalInput) Message() string {
	return ""
}

// SentAt returns message event's timestamp.
func (i *TransactionalInput) SentAt() time.Time {
	return i.TimeStamp
}

// ReplyTo returns replying destination.
func (i *TransactionalInput) ReplyTo() sarah.OutputDestination {
	return ConnectedOutputDestination
}

// NewResponse creates *sarah.CommandResponse with given iot.Role and payload content.
//
// This method is seldom used because usually iot.server initiates requests and device responds,
// which means transaction id is usually given as part of input and is to be used as part of response.
// NewTransactionResponse should be used for such case.
func NewResponse(role *iot.Role, payloadContent interface{}) *sarah.CommandResponse {
	return &sarah.CommandResponse{
		UserContext: nil,
		Content: &outputContent{
			role:           role,
			payloadContent: payloadContent,
		},
	}
}

// NewTransactionResponse can be used to build sarah.CommandResponse with given clues.
// When this is returned by sarah.Command, this object is internally used to construct responding payload.
func NewTransactionResponse(input *TransactionalInput, role *iot.Role, payloadContent interface{}) *sarah.CommandResponse {
	return &sarah.CommandResponse{
		UserContext: nil,
		Content: &outputContent{
			transactionID:  input.TransactionID,
			role:           role,
			payloadContent: payloadContent,
		},
	}
}

// NewOutput constructs sarah.Output with given arguments.
// This can be used as part of returning value of ScheduledTask.
func NewOutput(role *iot.Role, payloadContent interface{}) sarah.Output {
	return sarah.NewOutputMessage(
		ConnectedOutputDestination,
		&outputContent{
			role:           role,
			payloadContent: payloadContent,
		})
}

type outputContent struct {
	role           *iot.Role
	payloadContent interface{}

	// This field is set if and only if the outgoing payload represents a transactional output, which is a response to given request.
	transactionID string
}

// ToTransactionalInput is a utility method that tries to convert given sarah.Input to *device.TransactionalInput.
// In typical usage, when the input is *device.TransactionalInput, returning *sarah.CommandResponse should be constructed with
// device.NewTransactionResponse to return transactional response.
//
// func CommandFunc(i context.Context, input sarah.Input) (*sarah.CommandResponse, error) {
//     ti, ok := device.ToTransactionalInput(input)
//     if ok {
//	       return device.NewTransactionResponse(ti, ..., ...), nil
//     }
//
//     return ...
// }
func ToTransactionalInput(input sarah.Input) (*TransactionalInput, bool) {
	ti, ok := input.(*TransactionalInput)
	return ti, ok
}

// ToInput is a utility method that tries to convert given sarah.Input to *device.Input.
// In typical usage, when the input is *device.Input, returning *sarah.CommandResponse should be constructed with
// device.NewOutput to send regular output.
//
// func CommandFunc(i context.Context, input sarah.Input) (*sarah.CommandResponse, error) {
//     i, ok := device.ToInput(input)
//     if ok {
//	       return device.NewOutput(ti, ..., ...), nil
//     }
//
//     return ...
// }
func ToInput(input sarah.Input) (*Input, bool) {
	i, ok := input.(*Input)
	return i, ok
}
