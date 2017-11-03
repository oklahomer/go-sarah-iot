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
	"github.com/oklahomer/go-sarah-iot/payload"
	"golang.org/x/net/context"
)

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
	CloseFunc      func() error
	NextReaderFunc func() (int, io.Reader, error)
	PongFunc       func() error
	DeviceFunc     func() *Device
}

func (d *DummyConnection) Close() error {
	return d.CloseFunc()
}

func (d *DummyConnection) NextReader() (int, io.Reader, error) {
	return d.NextReaderFunc()
}

func (d *DummyConnection) Pong() error {
	return d.PongFunc()
}

func (d *DummyConnection) Device() *Device {
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

	if typed.upgrader == nil {
		t.Errorf("Default *websocket.Upgrader must be set when one is not supplied with AdapterOption.")
	}

	if typed.decoder == nil {
		t.Errorf("Default payload.Decoder must be set when one is not supplied with AdapterOption.")
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
	device := &Device{}
	authorizer := &DummyAuthorizer{
		AuthorizeFunc: func(_ *http.Request) (*Device, error) {
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
			AuthorizeFunc: func(_ *http.Request) (*Device, error) {
				return &Device{}, nil
			},
		},
	}

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

func TestInput_Message(t *testing.T) {
	input := &Input{}

	if input.Message() != "" {
		t.Fatalf("Unexpected message value is returned: %s.", input.Message())
	}
}

func TestInput_ReplyTo(t *testing.T) {
	device := &Device{}
	input := &Input{
		Device: device,
	}

	if input.ReplyTo() != device {
		t.Fatalf("Unexpected sarah.OutputDestination is returned: %+v.", input.ReplyTo())
	}
}

func TestInput_SenderKey(t *testing.T) {
	id := "id"
	device := &Device{
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
	device := &Device{}
	input := &ResponseInput{
		Device: device,
	}

	if input.ReplyTo() != device {
		t.Fatalf("Unexpected sarah.OutputDestination is returned: %+v.", input.ReplyTo())
	}
}

func TestResponseInput_SenderKey(t *testing.T) {
	id := "id"
	device := &Device{
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
			DeviceFunc: func() *Device {
				return &Device{}
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
			DeviceFunc: func() *Device {
				return &Device{}
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
		DeviceFunc: func() *Device {
			return &Device{
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
			Payload: &payload.ResponsePayload{
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
			DeviceFunc: func() *Device {
				return &Device{}
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

func Test_receivePayload_CloseMessage(t *testing.T) {
	conn := &DummyConnection{
		DeviceFunc: func() *Device {
			return &Device{}
		},
		NextReaderFunc: func() (int, io.Reader, error) {
			return websocket.CloseMessage, nil, nil
		},
	}

	a := &adapter{}
	err := a.receivePayload(conn, func(_ sarah.Input) error { return nil })

	if err != nil {
		t.Errorf("Unexpected error is rerturned: %s.", err.Error())
	}
}
