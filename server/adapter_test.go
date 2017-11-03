package server

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"

	"github.com/getlantern/httptest"
	"github.com/gorilla/websocket"
	"github.com/oklahomer/go-sarah"
	"golang.org/x/net/context"
)

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
