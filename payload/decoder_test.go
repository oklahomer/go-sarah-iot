package payload

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/oklahomer/go-sarah-iot"
)

func TestNewDecoder(t *testing.T) {
	decoder := NewDecoder()
	if decoder == nil {
		t.Fatal("Decoder instance is not returned.")
	}

	_, ok := decoder.(*defaultDecoder)
	if !ok {
		t.Fatalf("Returned value is not defaultDecoder's instance: %T.", decoder)
	}
}

func TestDefaultDecoder_AddMapping(t *testing.T) {
	type mappingType struct{}
	role := iot.NewRole("role")
	decoder := &defaultDecoder{
		mapper: &payloadMapper{
			m: map[*iot.Role]reflect.Type{},
		},
	}

	decoder.AddMapping(role, reflect.TypeOf(mappingType{}))
	val, ok := decoder.mapper.m[role]
	if !ok {
		t.Fatal("Mapping was not added.")
	}

	if val != reflect.TypeOf(mappingType{}) {
		t.Errorf("Expected type was not added: %v.", val)
	}
}

func TestDefaultDecoder_Decode(t *testing.T) {
	type content struct {
		Status string `json:"status"`
	}

	role := iot.NewRole("StatusChecker")
	str := `{"type": "roled", "role": "StatusChecker", "content": {"status": "OK"}}`

	for i, rt := range []reflect.Type{reflect.TypeOf(content{}), reflect.TypeOf(&content{})} {
		i++

		decoder := &defaultDecoder{
			mapper: &payloadMapper{
				m: map[*iot.Role]reflect.Type{
					role: rt,
				},
			},
		}

		p, err := decoder.Decode(websocket.TextMessage, strings.NewReader(str))

		if err != nil {
			t.Errorf("Unexpected error is returned on test #%d: %s.", i, err.Error())
		}

		roledPayload, ok := p.(*RoledPayload)
		if !ok {
			t.Fatalf("Unexpected type is returned on test #%d: %+v.", i, p)
		}

		if roledPayload.Role.String() != role.String() {
			t.Errorf("Unexpected role value is set: %+v", roledPayload)
		}

		c, ok := roledPayload.Content.(*content)
		if !ok {
			t.Fatalf("Unexpected content type is returned on test #%d: %+v.", i, p)
		}

		if c.Status != "OK" {
			t.Errorf("Unexpected status value is set on test #%d: %s.", i, c.Status)
		}
	}
}

func TestDefaultDecoder_Decode_TransactionalPayload(t *testing.T) {
	type content struct {
		Status string `json:"status"`
	}

	role := iot.NewRole("StatusChecker")
	str := `{"type": "roled", "transaction_id": "tid", "role": "StatusChecker", "content": {"status": "OK"}}`

	for i, rt := range []reflect.Type{reflect.TypeOf(content{}), reflect.TypeOf(&content{})} {
		i++

		decoder := &defaultDecoder{
			mapper: &payloadMapper{
				m: map[*iot.Role]reflect.Type{
					role: rt,
				},
			},
		}

		p, err := decoder.Decode(websocket.TextMessage, strings.NewReader(str))

		if err != nil {
			t.Errorf("Unexpected error is returned on test #%d: %s.", i, err.Error())
		}

		transactionalPayload, ok := p.(*TransactionalPayload)
		if !ok {
			t.Fatalf("Unexpected type is returned on test #%d: %+v.", i, p)
		}

		c, ok := transactionalPayload.Content.(*content)
		if !ok {
			t.Fatalf("Unexpected content type is returned on test #%d: %+v.", i, p)
		}

		if c.Status != "OK" {
			t.Errorf("Unexpected status value is set on test #%d: %s.", i, c.Status)
		}
	}
}

func TestDefaultDecoder_Decode_InvalidMessageType(t *testing.T) {
	decoder := &defaultDecoder{}

	str := `{"type": "roled", "role": "StatusChecker", "status": "OK"}`
	p, err := decoder.Decode(websocket.CloseMessage, strings.NewReader(str))

	if err == nil {
		t.Fatal("Expected error is not retruned.")
	}

	if p != nil {
		t.Errorf("Decoded payload is returned: %+v.", p)
	}
}

func TestDefaultDecoder_Decode_WithoutTypeField(t *testing.T) {
	decoder := &defaultDecoder{}

	str := `{"role": "StatusChecker", "status": "OK"}`
	p, err := decoder.Decode(websocket.TextMessage, strings.NewReader(str))

	if err == nil {
		t.Fatal("Expected error is not retruned.")
	}

	if p != nil {
		t.Errorf("Decoded payload is returned: %+v.", p)
	}
}

func TestDefaultDecoder_Decode_UnsupportedType(t *testing.T) {
	decoder := &defaultDecoder{}

	str := `{"type": "FOO", "status": "OK"}`
	p, err := decoder.Decode(websocket.TextMessage, strings.NewReader(str))
	if err == nil {
		t.Fatal("Expected error is not returned.")
	}

	if p != nil {
		t.Errorf("Unexpected payload is returned: %+v.", p)
	}
}

func TestDefaultDecoder_DecodeTyped(t *testing.T) {
	type content struct {
		Status string `json:"status"`
	}
	payload := []byte(`{"status": "OK"}`)

	for i, rt := range []reflect.Type{reflect.TypeOf(content{}), reflect.TypeOf(&content{})} {
		i++

		p, err := decodeTyped(payload, rt)

		if err != nil {
			t.Errorf("Unexpected error is returned on test #%d: %s.", i, err.Error())
		}

		decoded, ok := p.(*content)
		if !ok {
			t.Fatalf("Unexpected type is returned on test #%d: %+v.", i, p)
		}

		if decoded.Status != "OK" {
			t.Errorf("Unexpected status value is set on test #%d: %s.", i, decoded.Status)
		}
	}
}
