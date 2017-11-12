package server

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/getlantern/httptest"
	"github.com/gorilla/websocket"
	"github.com/oklahomer/go-sarah"
	"github.com/oklahomer/go-sarah-iot"
	"github.com/oklahomer/go-sarah-iot/auth"
	"github.com/oklahomer/go-sarah-iot/payload"
	"golang.org/x/net/context"
)

type DummyAuthorizer struct {
	AuthorizeFunc func(*http.Request) (*auth.Device, error)
}

func (d *DummyAuthorizer) Authorize(r *http.Request) (*auth.Device, error) {
	return d.AuthorizeFunc(r)
}

type DummyEncoder struct {
	EncodeFunc              func(string, interface{}) (int, []byte, error)
	EncodeRoledFunc         func(*iot.Role, interface{}) (int, []byte, error)
	EncodeTransactionalFunc func(string, *iot.Role, interface{}) (int, []byte, error)
}

func (de *DummyEncoder) Encode(payloadType string, content interface{}) (int, []byte, error) {
	return de.EncodeFunc(payloadType, content)
}

func (de *DummyEncoder) EncodeRoled(role *iot.Role, content interface{}) (int, []byte, error) {
	return de.EncodeRoledFunc(role, content)
}

func (de *DummyEncoder) EncodeTransactional(tid string, role *iot.Role, content interface{}) (int, []byte, error) {
	return de.EncodeTransactionalFunc(tid, role, content)
}

type DummyDecoder struct {
	AddMappingFunc func(*iot.Role, reflect.Type)
	DecodeFunc     func(int, io.Reader) (payload.DecodedPayload, error)
}

func (d *DummyDecoder) AddMapping(r *iot.Role, t reflect.Type) {
	d.AddMappingFunc(r, t)
}

func (d *DummyDecoder) Decode(messageType int, reader io.Reader) (payload.DecodedPayload, error) {
	return d.DecodeFunc(messageType, reader)
}

type DummyConnection struct {
	CloseFunc        func() error
	NextReaderFunc   func() (int, io.Reader, error)
	WriteMessageFunc func(int, []byte) error
	PongFunc         func() error
	DeviceFunc       func() *auth.Device
}

func (d *DummyConnection) Close() error {
	return d.CloseFunc()
}

func (d *DummyConnection) NextReader() (int, io.Reader, error) {
	return d.NextReaderFunc()
}

func (d *DummyConnection) WriteMessage(messageType int, output []byte) error {
	return d.WriteMessageFunc(messageType, output)
}

func (d *DummyConnection) Pong() error {
	return d.PongFunc()
}

func (d *DummyConnection) Device() *auth.Device {
	return d.DeviceFunc()
}

func TestWithDecoder(t *testing.T) {
	decoder := &DummyDecoder{}
	opt := WithDecoder(decoder)
	adapter := &adapter{}

	err := opt(adapter)

	if err != nil {
		t.Fatalf("AdapterOption returned an error: %s.", err.Error)
	}

	if adapter.decoder != decoder {
		t.Fatalf("Given payload.Decoder is not set: %+v", adapter.decoder)
	}
}

func TestWithEncoder(t *testing.T) {
	encoder := &DummyEncoder{}
	opt := WithEncoder(encoder)
	adapter := &adapter{}

	err := opt(adapter)

	if err != nil {
		t.Fatalf("AdapterOption returned an error: %s.", err.Error)
	}

	if adapter.encoder != encoder {
		t.Fatalf("Given payload.Encoder is not set: %+v", adapter.encoder)
	}
}

func TestWithUpgrader(t *testing.T) {
	upgrader := &websocket.Upgrader{}
	opt := WithUpgrader(upgrader)
	adapter := &adapter{}

	err := opt(adapter)

	if err != nil {
		t.Fatalf("AdapterOption returned an error: %s.", err.Error)
	}

	if adapter.upgrader != upgrader {
		t.Fatalf("Given *websocket.Upgrader is not set: %+v", adapter.upgrader)
	}
}

func TestWithTransactionStorage(t *testing.T) {
	storage := &transactionStorage{}
	opt := WithTransactionStorage(storage)
	adapter := &adapter{}

	err := opt(adapter)

	if err != nil {
		t.Fatalf("AdapterOption returned an error: %s.", err.Error)
	}

	if adapter.storage != storage {
		t.Fatalf("Given *websocket.Upgrader is not set: %+v", adapter.upgrader)
	}
}

func TestNewConfig(t *testing.T) {
	c := NewConfig()

	if c == nil {
		t.Fatal("Pointer to Config is not returned.")
	}

	if c.WebSocketPath == "" {
		t.Error("WebSocketPath is empty.")
	}

	if c.ListenPort == 0 {
		t.Error("ListenPort is 0.")
	}
}

func TestNewAdapter(t *testing.T) {
	c := &Config{}
	authorizer := &DummyAuthorizer{}
	a, err := NewAdapter(c, authorizer)

	if err != nil {
		t.Fatalf("Failed to initialize adapter: %s", err.Error())
	}

	typed, ok := a.(*adapter)
	if !ok {
		t.Fatalf("Pointer to adapter is not returned: %T", a)
	}

	if typed.config != c {
		t.Errorf("Given *Config is not set: %+v", typed.config)
	}

	if typed.authorizer != authorizer {
		t.Errorf("Given Authorizer is not set: %+v", typed.authorizer)
	}

	if typed.output == nil {
		t.Error("Channel for sarah.Output is not set.")
	}

	if typed.upgrader == nil {
		t.Errorf("Default *websocket.Upgrader must be set when one is not supplied with AdapterOption.")
	}

	if typed.decoder == nil {
		t.Errorf("Default payload.Decoder must be set when one is not supplied with AdapterOption.")
	}

	if typed.storage == nil {
		t.Errorf("Default TransactionStorage must be set when one is not supplied with AdapterOption.")
	}
}

func TestNewAdapter_WithAdapterOption(t *testing.T) {
	upgrader := &websocket.Upgrader{}
	a, err := NewAdapter(&Config{}, &DummyAuthorizer{}, WithUpgrader(upgrader))

	if err != nil {
		t.Fatalf("Failed to initialize adapter: %s", err.Error())
	}

	typed, ok := a.(*adapter)
	if !ok {
		t.Fatalf("Pointer to adapter is not returned: %T", a)
	}

	if typed.upgrader != upgrader {
		t.Errorf("Given *websocket.Upgrader is not set: %+v", typed.upgrader)
	}
}

func TestNewAdapter_WithErrorProneAdapterOption(t *testing.T) {
	errishOption := func(_ *adapter) error {
		return errors.New("boo")
	}
	_, err := NewAdapter(&Config{}, &DummyAuthorizer{}, errishOption)

	if err == nil {
		t.Fatalf("Expected error is not returned.")
	}
}

func TestAdapter_BotType(t *testing.T) {
	a := &adapter{}
	if a.BotType() != IoTServer {
		t.Errorf("Expected BotType is not returned: %s.", a.BotType())
	}
}

func Test_serverHandleFunc(t *testing.T) {
	// Prepare handler func
	device := &auth.Device{}
	authorizer := &DummyAuthorizer{
		AuthorizeFunc: func(_ *http.Request) (*auth.Device, error) {
			return device, nil
		},
	}
	upgrader := &websocket.Upgrader{}
	c := make(chan connection, 1)
	fnc := serverHandleFunc(authorizer, upgrader, c)

	// Prepare dummy request
	req, err := http.NewRequest("GET", "/dummy", nil)
	if err != nil {
		t.Fatalf("Error on preparing testing argument: %s.", err.Error())
	}
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "upgrade")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "E4WSEcseoWr4csPLS2QJHA==")

	// Prepare ResponseWriter implementation
	writer := httptest.NewRecorder(nil)

	// Here comes the test
	fnc(writer, req)

	select {
	case conn := <-c:
		typedConn, ok := conn.(*connWrapper)
		if !ok {
			t.Fatalf("Given connection is not type of *connWrapper: %T.", conn)
		}

		if typedConn.conn == nil {
			t.Error("Nil conn is set.")
		}

		if typedConn.device != device {
			t.Errorf("Expected Device is not set: %#v.", typedConn.device)
		}

	default:
		t.Fatalf("Expected connection is not given.")
	}
}

func TestAdapter_Run(t *testing.T) {
	// Find usable port
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to resolve usable port number: %s.", err.Error())
	}
	listener, err := net.ListenTCP("tcp", addr)
	if err != nil {
		t.Fatalf("Failed to Listen port: %s.", err.Error())
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	// Setup adapter
	path := "/test"
	config := &Config{
		WebSocketPath: path,
		ListenPort:    port,
	}
	adapter := &adapter{
		config:   config,
		upgrader: &websocket.Upgrader{},
		authorizer: &DummyAuthorizer{
			AuthorizeFunc: func(_ *http.Request) (*auth.Device, error) {
				return &auth.Device{}, nil
			},
		}}

	// Run adapter
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	givenError := make(chan error, 1)
	errNotifier := func(err error) {
		givenError <- err
	}
	go adapter.Run(ctx, func(_ sarah.Input) error { return nil }, errNotifier)

	// Try establish WebSocket connection
	header := http.Header{}
	dialer := websocket.Dialer{}
	conn, res, err := dialer.Dial(fmt.Sprintf("ws://localhost:%d/test", port), header)
	// Make sure to declare defer statement before calling t.Fatalf.
	// This is not meant to be part of test assertion.
	if conn != nil {
		defer conn.Close()
	}

	// Some assertion follows

	if err != nil {
		t.Fatalf("Error on connection establishing: %s.", err.Error())
	}

	if res.StatusCode == 200 {
		t.Fatalf("Expected status code is not returned: %d", res.StatusCode)
	}

	if conn == nil {
		t.Fatalf("Connection is not returned.")
	}
}

type dummyOutput struct {
}

func (*dummyOutput) Destination() sarah.OutputDestination {
	panic("implement me")
}

func (*dummyOutput) Content() interface{} {
	panic("implement me")
}

func TestAdapter_SendMessage(t *testing.T) {
	outputs := []sarah.Output{
		&transactionalOutput{},
		&output{},
		&dummyOutput{}, // Not subject to enqueue via adapter.output.
	}

	a := &adapter{
		output: make(chan sarah.Output, len(outputs)),
	}

	for _, output := range outputs {
		a.SendMessage(context.TODO(), output)
	}

	if len(a.output) != 2 {
		t.Errorf("Unexpected number of sarah.Output instances are enqueued: %d.", len(a.output))
	}
}

func TestInput_Message(t *testing.T) {
	input := &Input{}

	if input.Message() != "" {
		t.Fatalf("Unexpected message value is returned: %s.", input.Message())
	}
}

func TestInput_ReplyTo(t *testing.T) {
	device := &auth.Device{}
	input := &Input{
		Device: device,
	}

	if input.ReplyTo() != device {
		t.Fatalf("Unexpected sarah.OutputDestination is returned: %+v.", input.ReplyTo())
	}
}

func TestInput_SenderKey(t *testing.T) {
	id := "id"
	device := &auth.Device{
		ID: id,
	}
	input := &Input{
		Device: device,
	}

	if input.SenderKey() != id {
		t.Fatalf("Unexpected SenderKey is returned: %+v.", input.SenderKey())
	}
}

func TestInput_SentAt(t *testing.T) {
	now := time.Now()
	input := &Input{
		TimeStamp: now,
	}

	if input.SentAt() != now {
		t.Fatalf("Unexpected timestamp is returned: %s", now.String())
	}
}

func TestResponseInput_Message(t *testing.T) {
	input := &ResponseInput{}

	if input.Message() != "" {
		t.Fatalf("Unexpected message value is returned: %s.", input.Message())
	}
}

func TestResponseInput_ReplyTo(t *testing.T) {
	device := &auth.Device{}
	input := &ResponseInput{
		Device: device,
	}

	if input.ReplyTo() != device {
		t.Fatalf("Unexpected sarah.OutputDestination is returned: %+v.", input.ReplyTo())
	}
}

func TestResponseInput_SenderKey(t *testing.T) {
	id := "id"
	device := &auth.Device{
		ID: id,
	}
	input := &ResponseInput{
		Device: device,
	}

	if input.SenderKey() != id {
		t.Fatalf("Unexpected SenderKey is returned: %+v.", input.SenderKey())
	}
}

func TestResponseInput_SentAt(t *testing.T) {
	now := time.Now()
	input := &ResponseInput{
		TimeStamp: now,
	}

	if input.SentAt() != now {
		t.Fatalf("Unexpected timestamp is returned: %s", now.String())
	}
}

func Test_superviseConnection_ContextClose(t *testing.T) {
	a := &adapter{
		decoder: &DummyDecoder{
			DecodeFunc: func(_ int, _ io.Reader) (payload.DecodedPayload, error) {
				return nil, errors.New("dummy")
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	incoming := make(chan connection)
	finished := make(chan struct{}, 1)
	go func() {
		a.superviseConnection(ctx, incoming, func(sarah.Input) error { return nil })
		finished <- struct{}{}
	}()

	closed := make(chan struct{}, 2)
	conns := []connection{
		&DummyConnection{
			CloseFunc: func() error {
				closed <- struct{}{}
				return nil
			},
			DeviceFunc: func() *auth.Device {
				return &auth.Device{}
			},
			NextReaderFunc: func() (int, io.Reader, error) {
				return websocket.TextMessage, strings.NewReader(`{"type": "roled", "role": "foo", "content": {}}`), nil
			},
		},
		&DummyConnection{
			CloseFunc: func() error {
				closed <- struct{}{}
				return nil
			},
			DeviceFunc: func() *auth.Device {
				return &auth.Device{}
			},
			NextReaderFunc: func() (int, io.Reader, error) {
				return websocket.TextMessage, strings.NewReader(`{"type": "roled", "role": "foo", "content": {}}`), nil
			},
		},
	}

	for _, conn := range conns {
		incoming <- conn
	}

	cancel()

	<-time.NewTimer(100 * time.Millisecond).C

	if len(closed) != len(conns) {
		t.Errorf("All belonging connection must close.")
	}

	select {
	case <-finished:
		// O.K.

	default:
		t.Errorf("superviseConnection is not finished.")

	}
}

func Test_superviseConnection_ErrorOnConnectionRead(t *testing.T) {
	a := &adapter{
		decoder: &DummyDecoder{
			DecodeFunc: func(_ int, _ io.Reader) (payload.DecodedPayload, error) {
				return nil, errors.New("dummy")
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	incoming := make(chan connection)
	go a.superviseConnection(ctx, incoming, func(sarah.Input) error { return nil })

	closed := make(chan struct{}, 2)
	conn := &DummyConnection{
		CloseFunc: func() error {
			closed <- struct{}{}
			return nil
		},
		DeviceFunc: func() *auth.Device {
			return &auth.Device{
				ID: "id",
			}
		},
		NextReaderFunc: func() (int, io.Reader, error) {
			return 0, nil, errors.New("dummy")
		},
	}
	incoming <- conn

	<-time.NewTimer(100 * time.Millisecond).C

	select {
	case <-closed:
		// O.K.

	default:
		t.Errorf("Connection is not closed on read error.")

	}
}

func Test_receivePayload(t *testing.T) {
	tests := []struct {
		Payload payload.DecodedPayload
		Reader  io.Reader
		Type    reflect.Type
	}{
		{
			Payload: &payload.RoledPayload{
				Content: struct{}{},
			},
			Reader: strings.NewReader(`{"type": "roled", "role": "foo", "content": {}}`),
			Type:   reflect.TypeOf(&Input{}),
		},
		{
			Payload: &payload.TransactionalPayload{
				TransactionID: "123",
				Content:       struct{}{},
			},
			Reader: strings.NewReader(`{"type": "roled", "role": "foo", "transaction_id": "123", "content": {}}`),
			Type:   reflect.TypeOf(&ResponseInput{}),
		},
	}

	for i, test := range tests {
		i++

		inputs := make(chan sarah.Input, 1)
		input := func(i sarah.Input) error {
			inputs <- i
			return nil
		}
		decoder := &DummyDecoder{
			DecodeFunc: func(_ int, _ io.Reader) (payload.DecodedPayload, error) {
				return test.Payload, nil
			},
		}
		a := &adapter{
			decoder: decoder,
		}

		conn := &DummyConnection{
			DeviceFunc: func() *auth.Device {
				return &auth.Device{}
			},
			NextReaderFunc: func() (int, io.Reader, error) {
				return websocket.TextMessage, test.Reader, nil
			},
		}

		go a.receivePayload(conn, input)

		<-time.NewTimer(100 * time.Millisecond).C

		select {
		case in := <-inputs:
			if reflect.TypeOf(in) != test.Type {
				t.Errorf("Expected type of payload is not returned on test #%d. Get %#v.", i, in)
			}

		default:
			t.Errorf("Expected input is not arrived.")

		}
	}
}

func Test_handleInput_Input(t *testing.T) {
	storage := &DummyStorage{}
	receiveInput := func(in sarah.Input) error {
		_, ok := in.(*Input)
		if ok {
			return nil
		}

		t.Errorf("Invalid implementation of sarah.Input is passed: %T.", in)
		return nil
	}

	err := handleInput(&Input{}, storage, receiveInput)

	if err != nil {
		t.Errorf("Unexpected error is returned: %s.", err.Error())
	}
}

func Test_handleInput_ResponseInput(t *testing.T) {
	tests := []struct {
		input  *ResponseInput
		stored bool
		block  bool
	}{
		{
			input:  &ResponseInput{},
			stored: true,
			block:  false,
		},
		{
			input:  &ResponseInput{},
			stored: false,
			block:  false,
		},
		{
			input: &ResponseInput{
				Device: &auth.Device{
					ID: "id",
				},
			},
			stored: true,
			block:  true,
		},
	}

	notStored := errors.New("")
	for i, test := range tests {
		var buffer int
		if !test.block {
			buffer = 1
		}
		responseCh := make(chan *Response, buffer)
		storage := &DummyStorage{
			GetFunc: func(_ string) (chan *Response, error) {
				if test.stored {
					return responseCh, nil
				} else {
					return nil, notStored
				}
			},
		}
		inputCh := make(chan sarah.Input, 1)
		receiveInput := func(in sarah.Input) error {
			_, ok := in.(*Input)
			if ok {
				inputCh <- in
				return nil
			}

			t.Errorf("Invalid implementation of sarah.Input is passed on test #%d: %T.", i, in)
			return nil
		}

		err := handleInput(test.input, storage, receiveInput)

		if !test.stored && err == nil {
			t.Errorf("Error should be returend when storage does not return channel on test #%d.", i)
		} else if test.block && err == nil {
			t.Errorf("Error should immediately be returned when channel is blocked on test #%d.", i)
		} else if test.stored && !test.block && err != nil {
			t.Errorf("Unexpected error is returned on test #%d: %s.", i, err.Error())
		} else if test.stored && !test.block && len(responseCh) != 1 {
			t.Errorf("Expected response is not enqueued on test #%d.", i)
		}
	}
}

func Test_encodeOutput(t *testing.T) {
	outputs := []sarah.Output{
		&output{
			destination: &Destination{
				role: &iot.Role{},
			},
		},
		&transactionalOutput{
			destination: &Destination{
				role: &iot.Role{},
			},
		},
		&dummyOutput{},
	}

	for i, out := range outputs {
		roledCalled := false
		transactionalCalled := false
		encoder := &DummyEncoder{
			EncodeRoledFunc: func(_ *iot.Role, _ interface{}) (int, []byte, error) {
				roledCalled = true
				return websocket.TextMessage, []byte(""), nil
			},
			EncodeTransactionalFunc: func(_ string, _ *iot.Role, _ interface{}) (int, []byte, error) {
				transactionalCalled = true
				return websocket.TextMessage, []byte(""), nil
			},
		}

		_, _, err := encodeOutput(out, encoder)

		switch out.(type) {
		case *output:
			if !roledCalled {
				t.Errorf("Encoder.EncodeRolled is not called on test #%d.", i)
			}

		case *transactionalOutput:
			if !transactionalCalled {
				t.Errorf("Encoder.EncodeTransactional is not called on test #%d.", i)
			}

		default:
			if err == nil {
				t.Errorf("Error should be returned when uncontrollable sarah.Output is fed on test #%d.", i)
			}

		}
	}
}

func Test_extractDestConns(t *testing.T) {
	tests := []struct {
		expectedN   int
		destination sarah.OutputDestination
		connections []connection
	}{
		{
			expectedN: 1,
			destination: &Destination{
				deviceID: "123",
			},
			connections: []connection{
				&DummyConnection{
					DeviceFunc: func() *auth.Device {
						return &auth.Device{ID: "invalid"}
					},
				},
				&DummyConnection{
					DeviceFunc: func() *auth.Device {
						return &auth.Device{ID: "123"}
					},
				},
				&DummyConnection{
					DeviceFunc: func() *auth.Device {
						return &auth.Device{ID: "123"} // Should be ignored due to ID duplication.
					},
				},
			},
		},
		{
			expectedN: 2,
			destination: &Destination{
				role: iot.NewRole("validRole"),
			},
			connections: []connection{
				&DummyConnection{
					DeviceFunc: func() *auth.Device {
						return &auth.Device{Roles: iot.Roles{iot.NewRole("validRole")}}
					},
				},
				&DummyConnection{
					DeviceFunc: func() *auth.Device {
						return &auth.Device{Roles: iot.Roles{iot.NewRole("invalid")}}
					},
				},
				&DummyConnection{
					DeviceFunc: func() *auth.Device {
						return &auth.Device{Roles: iot.Roles{iot.NewRole("validRole")}}
					},
				},
			},
		},
	}

	for i, test := range tests {
		conns := extractDestConns(test.destination, test.connections)

		if len(conns) != test.expectedN {
			t.Errorf("Unexpected size of connections are returned on test #%d: %d", i, len(conns))
		}
	}
}

func Test_awaitResponse(t *testing.T) {
	tests := []struct {
		expectedN int
		block     bool
		responses []*Response
	}{
		{
			expectedN: 0,
			block:     false,
			responses: []*Response{},
		},
		{
			expectedN: 2,
			block:     false,
			responses: []*Response{
				{},
				{},
				{}, // 3rd one should be just ignored.
			},
		},
		{
			expectedN: 2,
			block:     true,
			responses: []*Response{
				{},
				{},
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i, test := range tests {
		in := make(chan *Response, len(test.responses))
		var out chan []*Response
		if test.block {
			out = make(chan []*Response)
		} else {
			out = make(chan []*Response, 1)
		}

		go awaitResponse(ctx, test.expectedN, in, out)

		for _, res := range test.responses {
			in <- res
		}

		// To test blocking channel on test.block.
		<-time.NewTimer(100 * time.Millisecond).C

		select {
		case o := <-out:
			if test.expectedN != len(o) {
				t.Errorf("Expected number of output is not returned on test #%d: %d.", i, len(o))
			}

		default:
			if !test.block {
				t.Errorf("Failed to receive output ono test #%d.", i)
			}

		}
	}
}
