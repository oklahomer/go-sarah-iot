package payload

import (
	"encoding/json"

	"github.com/gorilla/websocket"
	"github.com/oklahomer/go-sarah-iot"
)

// Encoder represents an interface that encodes given WebSocket payload.
type Encoder interface {
	Encode(string, interface{}) (int, []byte, error)
	EncodeRoled(*iot.Role, interface{}) (int, []byte, error)
	EncodeTransactional(string, *iot.Role, interface{}) (int, []byte, error)
}

type defaultEncoder struct{}

var _ Encoder = (*defaultEncoder)(nil)

func (*defaultEncoder) Encode(payloadType string, content interface{}) (int, []byte, error) {
	output := &typedOutput{
		Type:    payloadType,
		Content: content,
	}
	p, err := json.Marshal(output)
	return websocket.TextMessage, p, err
}

func (*defaultEncoder) EncodeRoled(role *iot.Role, content interface{}) (int, []byte, error) {
	output := &roledOutput{
		Role:    role,
		Type:    "roled",
		Content: content,
	}
	p, err := json.Marshal(output)
	return websocket.TextMessage, p, err
}

func (*defaultEncoder) EncodeTransactional(tid string, role *iot.Role, content interface{}) (int, []byte, error) {
	output := &transactionalOutput{
		Role:    role,
		TID:     tid,
		Type:    "roled",
		Content: content,
	}
	p, err := json.Marshal(output)
	return websocket.TextMessage, p, err
}

// NewEncoder returns default implementation of Encoder, which uses JSON as its serialization format.
// To alter serialization/deserialization behavior, implement own Encoder/Decoder and replace default ones.
//
// Default output examples are as below:
//
// {"type": "ping"}
//
// {"type": "roled", "role": "thermo", "content": {"type": "scheduled_report", "temperature": 100}
//
// {"type": "roled", "role": "thermo", "transaction_id": "thermo123", "content": {"type": "response", "temperature": 100}}
func NewEncoder() Encoder {
	return &defaultEncoder{}
}

type typedOutput struct {
	Type    string      `json:"type"`
	Content interface{} `json:"content"`
}

type roledOutput struct {
	Role    *iot.Role   `json:"role"`
	Type    string      `json:"type"`
	Content interface{} `json:"content"`
}

type transactionalOutput struct {
	Role    *iot.Role   `json:"role"`
	TID     string      `json:"transaction_id"`
	Type    string      `json:"type"`
	Content interface{} `json:"content"`
}
