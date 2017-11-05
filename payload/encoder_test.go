package payload

import (
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/oklahomer/go-sarah-iot"
)

func TestNewEncoder(t *testing.T) {
	encoder := NewEncoder()
	if encoder == nil {
		t.Fatalf("Encoder instance is not returned.")
	}

	_, ok := encoder.(*defaultEncoder)
	if !ok {
		t.Fatalf("Returned value is not defaultEncoder's instance: %T.", encoder)
	}
}

func TestDefaultEncoder_Encode(t *testing.T) {
	content := struct {
		Foo []string `json:"foo"`
	}{
		Foo: []string{
			"bar",
			"buz",
		},
	}
	encoder := &defaultEncoder{}
	i, b, err := encoder.Encode("ping", content)

	if err != nil {
		t.Fatalf("Unexpected error is returened: %s.", err.Error())
	}

	if i != websocket.TextMessage {
		t.Errorf("Unexpxected messageType is returned: %d.", i)
	}

	s := string(b)
	for _, test := range []string{"type", "ping", "content", "foo", "bar", "buz"} {
		if !strings.Contains(s, test) {
			t.Errorf("Encded string does not contain %s: %s.", test, s)
		}
	}
}

func TestDefaultEncoder_EncodeRoled(t *testing.T) {
	content := struct {
		Value string `json:"value"`
	}{
		Value: "aeiou",
	}
	encoder := &defaultEncoder{}
	role := iot.NewRole("abc")
	i, b, err := encoder.EncodeRoled(role, content)

	if err != nil {
		t.Fatalf("Unexpected error is returened: %s.", err.Error())
	}

	if i != websocket.TextMessage {
		t.Errorf("Unexpxected messageType is returned: %d.", i)
	}

	s := string(b)
	for _, test := range []string{"type", "roled", "role", "abc", "content", "value", "aeiou"} {
		if !strings.Contains(s, test) {
			t.Errorf("Encded string does not contain %s: %s.", test, s)
		}
	}
}

func TestDefaultEncoder_EncodeTransactional(t *testing.T) {
	content := struct {
		Value string `json:"value"`
	}{
		Value: "aeiou",
	}
	encoder := &defaultEncoder{}
	role := iot.NewRole("abc")
	tid := "tid"

	i, b, err := encoder.EncodeTransactional(tid, role, content)

	if err != nil {
		t.Fatalf("Unexpected error is returened: %s.", err.Error())
	}

	if i != websocket.TextMessage {
		t.Errorf("Unexpxected messageType is returned: %d.", i)
	}

	s := string(b)
	for _, test := range []string{"type", "roled", "role", "abc", "transaction_id", "tid", "content", "value", "aeiou"} {
		if !strings.Contains(s, test) {
			t.Errorf("Encded string does not contain %s: %s.", test, s)
		}
	}
}
