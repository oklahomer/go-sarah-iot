package payload

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"reflect"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/oklahomer/go-sarah-iot"
	"github.com/tidwall/gjson"
)

// DecodedPayload represents an interface that all decoded payload must satisfy.
type DecodedPayload interface{}

// RoledPayload represents incoming payload with specific *iot.Role.
type RoledPayload struct {
	Role    *iot.Role
	Content interface{}
}

var _ DecodedPayload = (*RoledPayload)(nil)

// ResponosePayload represents incoming payload with transaction id.
// TODO rename this to TransactionalPayload since this can be part of both requesting and response payload.
type ResponsePayload struct {
	TransactionID string
	Content       interface{}
}

var _ DecodedPayload = (*ResponsePayload)(nil)

// Timestamper defines an interface that all payloads must implement to indicate sending time.
// IoT device may not have accurate time, so server.Adapter may use reception timestamp if incoming payload does not implement this interface.
type Timestamper interface {
	SentAt() time.Time
}

// Decoder represents an interface that decodes given WebSocket payload.
type Decoder interface {
	AddMapping(*iot.Role, reflect.Type)
	Decode(int, io.Reader) (DecodedPayload, error)
}

// NewDecoder creates new defaultDecoder and returns it as Decoder.
//
// This receives incoming payload in form of io.Reader, and tries to decode its body as JSON.
// To alter serialization/deserialization behavior, implement own Encoder/Decoder and replace default ones.
func NewDecoder() Decoder {
	return &defaultDecoder{
		mapper: &payloadMapper{
			m: map[*iot.Role]reflect.Type{},
		},
	}
}

type defaultDecoder struct {
	mapper *payloadMapper
}

var _ Decoder = (*defaultDecoder)(nil)

func (d *defaultDecoder) AddMapping(r *iot.Role, t reflect.Type) {
	d.mapper.Add(r, t)
}

func (d *defaultDecoder) Decode(messageType int, reader io.Reader) (DecodedPayload, error) {
	if messageType != websocket.TextMessage {
		return nil, fmt.Errorf("unexpected message type is given: %d", messageType)
	}

	body, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %s", err.Error())
	}

	res := gjson.ParseBytes(body)
	typeValue := res.Get("type")
	contentValue := res.Get("content")
	if !(typeValue.Exists() && contentValue.Exists()) {
		return nil, fmt.Errorf("both 'type' and 'content' fields must be present")
	}

	switch typeValue.String() {
	case "roled":
		roleValue := res.Get("role")
		if !roleValue.Exists() {
			return nil, fmt.Errorf("'role' field must be present when 'type' is 'roled': %s", res.String())
		}

		mapping, err := d.mapper.Get(roleValue.String())
		if err != nil {
			return nil, err
		}

		payload, err := decodeTyped([]byte(contentValue.String()), mapping)
		if err != nil {
			return nil, err
		}

		tidValue := res.Get("transaction_id")
		if tidValue.Exists() {
			return &ResponsePayload{
				TransactionID: tidValue.String(),
				Content:       payload,
			}, nil
		}

		return &RoledPayload{
			Role:    iot.NewRole(roleValue.String()),
			Content: payload,
		}, nil

	default:
		return nil, fmt.Errorf("unexpected type value is given: %s", typeValue.String())

	}
}

func decodeTyped(content []byte, mapping reflect.Type) (DecodedPayload, error) {
	// TODO check for other types needed? Map, Slice, etc...
	var payload interface{}
	if mapping.Kind() == reflect.Ptr {
		payload = reflect.New(mapping.Elem()).Interface()
	} else {
		payload = reflect.New(mapping).Interface()
	}

	// gjson.Unmarshal does not seem to handle embedded struct properly.
	err := json.Unmarshal(content, &payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode payload: %s", err.Error())
	}

	return payload, nil
}

type payloadMapper struct {
	m     map[*iot.Role]reflect.Type
	mutex sync.RWMutex
}

func (mapper *payloadMapper) Add(role *iot.Role, payloadType reflect.Type) {
	mapper.mutex.Lock()
	defer mapper.mutex.Unlock()
	mapper.m[role] = payloadType
}

func (mapper *payloadMapper) Get(r string) (reflect.Type, error) {
	mapper.mutex.RLock()
	defer mapper.mutex.RUnlock()

	for k, v := range mapper.m {
		if k.String() == r {
			return v, nil
		}
	}

	return nil, fmt.Errorf("corresponoding type is not registered for %s", r)
}
