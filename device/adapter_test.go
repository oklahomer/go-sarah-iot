package device

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/oklahomer/go-sarah"
	"github.com/oklahomer/go-sarah-iot"
	"github.com/oklahomer/go-sarah-iot/payload"
)

type DummyConnection struct {
	CloseFunc        func() error
	NextReaderFunc   func() (int, io.Reader, error)
	WriteMessageFunc func(int, []byte) error
	PongFunc         func() error
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

func TestNewConfig(t *testing.T) {
	c := NewConfig()

	if c == nil {
		t.Fatal("Pointer to Config is not returned.")
	}
}

func TestWithDecoder(t *testing.T) {
	decoder := &DummyDecoder{}
	opt := WithDecoder(decoder)
	adapter := &adapter{}

	err := opt(adapter)

	if err != nil {
		t.Fatalf("AdapterOption returned an error: %s.", err.Error())
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
		t.Fatalf("AdapterOption returned an error: %s.", err.Error())
	}

	if adapter.encoder != encoder {
		t.Fatalf("Given payload.Encoder is not set: %+v", adapter.encoder)
	}
}

func TestNewAdapter(t *testing.T) {
	c := &Config{}
	a, err := NewAdapter(c)

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

	if typed.decoder == nil {
		t.Errorf("Default payload.Decoder must be set when one is not supplied with AdapterOption.")
	}
}

func TestNewAdapter_WithAdapterOption(t *testing.T) {
	decoder := &DummyDecoder{}
	a, err := NewAdapter(&Config{}, WithDecoder(decoder))

	if err != nil {
		t.Fatalf("Failed to initialize adapter: %s", err.Error())
	}

	typed, ok := a.(*adapter)
	if !ok {
		t.Fatalf("Pointer to adapter is not returned: %T", a)
	}

	if typed.decoder != decoder {
		t.Errorf("Given payload.Decoder is not set: %+v", typed.decoder)
	}
}

func TestNewAdapter_WithErrorProneAdapterOption(t *testing.T) {
	errishOption := func(_ *adapter) error {
		return errors.New("boo")
	}
	_, err := NewAdapter(&Config{}, errishOption)

	if err == nil {
		t.Fatalf("Expected error is not returned.")
	}
}

func TestAdapter_BotType(t *testing.T) {
	a := &adapter{}
	if a.BotType() != IoTDevice {
		t.Errorf("Expected BotType is not returned: %s.", a.BotType())
	}
}

func TestAdapter_SendMessage(t *testing.T) {
	outputs := []sarah.Output{
		sarah.NewOutputMessage(struct{}{}, struct{}{}), // invalid
		NewOutput(iot.NewRole("dummy"), &outputContent{}),
	}
	a := &adapter{
		output: make(chan *outputContent, len(outputs)),
	}

	for _, output := range outputs {
		a.SendMessage(context.TODO(), output)
	}

	if len(a.output) != 1 {
		t.Fatalf("Unexpected size of paylaods are queued: %d.", len(a.output))
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
			Type:   reflect.TypeOf(&TransactionalInput{}),
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
			config: &Config{
				ServerURL: "ws://localhost/dummy",
			},
		}

		conn := &DummyConnection{
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

func TestNewOutput(t *testing.T) {
	role := iot.NewRole("dummy")
	content := struct{}{}

	output := NewOutput(role, content)

	if output == nil {
		t.Fatal("Expected sarah.Output instance is not returned.")
	}

	typed, ok := output.Content().(*outputContent)
	if !ok {
		t.Fatalf("Expected type of content is not set: %T.", typed)
	}

	if typed.role != role {
		t.Errorf("Expected *iot.Role is not set: %+v", typed.role)
	}

	if typed.payloadContent != content {
		t.Errorf("Expected content is not set: %+v", typed.payloadContent)
	}

	if typed.transactionID != "" {
		t.Errorf("Unexpected transaction id is set: %s.", typed.transactionID)
	}
}

func TestNewResponse(t *testing.T) {
	role := iot.NewRole("dummy")
	content := struct{}{}

	response := NewResponse(role, content)

	if response == nil {
		t.Fatal("Expected *sarah.CommandResponse is not returned.")
	}

	if response.UserContext != nil {
		t.Fatalf("Unexpected *sarah.UserContext is set: %+v.", response.UserContext)
	}

	typed, ok := response.Content.(*outputContent)
	if !ok {
		t.Fatalf("Expected type of content is not set: %T.", typed)
	}

	if typed.role != role {
		t.Errorf("Expected *iot.Role is not set: %+v", typed.role)
	}

	if typed.payloadContent != content {
		t.Errorf("Expected content is not set: %+v", typed.payloadContent)
	}

	if typed.transactionID != "" {
		t.Errorf("Unexpected transaction id is set: %s.", typed.transactionID)
	}
}

func TestNewTransactionResponse(t *testing.T) {
	input := &TransactionalInput{
		TransactionID: "id",
	}
	role := iot.NewRole("dummy")
	content := struct{}{}

	response := NewTransactionResponse(input, role, content)

	if response == nil {
		t.Fatal("Expected *sarah.CommandResponse is not returned.")
	}

	if response.UserContext != nil {
		t.Fatalf("Unexpected *sarah.UserContext is set: %+v.", response.UserContext)
	}

	typed, ok := response.Content.(*outputContent)
	if !ok {
		t.Fatalf("Expected type of content is not set: %T.", typed)
	}

	if typed.role != role {
		t.Errorf("Expected *iot.Role is not set: %+v", typed.role)
	}

	if typed.payloadContent != content {
		t.Errorf("Expected content is not set: %+v", typed.payloadContent)
	}

	if typed.transactionID == "" {
		t.Error("Expected transaction id is not set.")
	}
}

func Test_encodeOutput(t *testing.T) {
	outputs := []*outputContent{
		{
			role:           &iot.Role{},
			payloadContent: struct{}{},
		},
		{
			role:           &iot.Role{},
			payloadContent: struct{}{},
			transactionID:  "dummy",
		},
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

		if err != nil {
			t.Fatalf("Unexpected error is returned: %s.", err.Error())
		}

		if out.transactionID == "" {
			if !roledCalled {
				t.Errorf("Encoder.EncodeRolled is not called on test #%d.", i)
			}
		} else {
			if !transactionalCalled {
				t.Errorf("Encoder.EncodeTransactional is not called on test #%d.", i)
			}
		}
	}
}

func TestToTransactionalInput(t *testing.T) {
	input := &TransactionalInput{}

	ti, ok := ToTransactionalInput(input)

	if !ok {
		t.Error("Failed to cast type.")
	}

	if ti == nil {
		t.Error("Returned value is nil.")
	}
}

func TestToInput(t *testing.T) {
	input := &Input{}

	i, ok := ToInput(input)

	if !ok {
		t.Error("Failed to cast type.")
	}

	if i == nil {
		t.Error("Returned value is nil.")
	}
}
