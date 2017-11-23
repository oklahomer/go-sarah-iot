package server

import (
	"testing"

	"github.com/oklahomer/go-sarah-iot"
)

func TestNewOutput(t *testing.T) {
	destination := &Destination{}
	content := struct{}{}

	o := NewOutput(destination, content)

	if o == nil {
		t.Fatalf("Returned Output is nil.")
	}

	typed, ok := o.(*output)
	if !(ok) {
		t.Fatalf("Given sarah.Output is not *output: %T.", o)
	}

	if typed.destination != destination {
		t.Errorf("Invalid destination is set: %+v.", typed.destination)
	}

	if typed.content != content {
		t.Errorf("Invalid content is set: %+v.", typed.content)
	}
}

func TestOutput_Destination(t *testing.T) {
	destination := &Destination{}
	o := &output{
		destination: destination,
	}

	if o.Destination() != destination {
		t.Fatalf("Unexpected sarah.Destination is returned: %T.", o.Destination())
	}
}

func TestOutput_Content(t *testing.T) {
	content := struct{}{}
	o := &output{
		content: content,
	}

	if o.Content() != content {
		t.Fatalf("Unexpected content is returned: %T.", o.Content())
	}
}

func TestNewTransactionalOutput(t *testing.T) {
	tid := "123"
	destination := &Destination{}
	content := struct{}{}

	o, _ := NewTransactionalOutput(tid, destination, content)

	if o == nil {
		t.Fatalf("Returned Output is nil.")
	}

	typed, ok := o.(*transactionalOutput)
	if !(ok) {
		t.Fatalf("Given sarah.Output is not *output: %T.", o)
	}

	if typed.destination != destination {
		t.Errorf("Invalid destination is set: %+v.", typed.destination)
	}

	if typed.content != content {
		t.Errorf("Invalid content is set: %+v.", typed.content)
	}

	if typed.transactionID != tid {
		t.Errorf("Invalid transaction id is set: %s.", typed.transactionID)
	}
}

func TestTransactionalOutput_Destination(t *testing.T) {
	destination := &Destination{}
	o := &transactionalOutput{
		destination: destination,
	}

	if o.Destination() != destination {
		t.Fatalf("Unexpected sarah.Destination is returned: %T.", o.Destination())
	}
}

func TestTransactionalOutput_Content(t *testing.T) {
	content := struct{}{}
	o := &transactionalOutput{
		content: content,
	}

	if o.Content() != content {
		t.Fatalf("Unexpected content is returned: %T.", o.Content())
	}
}

func TestNewDestinationBuilder(t *testing.T) {
	builder := NewDestinationBuilder()

	if builder == nil {
		t.Errorf("Nil is returned.")
	}
}

func TestDestinationBuilder_Role(t *testing.T) {
	role := iot.NewRole("role")
	builder := &DestinationBuilder{}

	builder.Role(role)

	if builder.role != role {
		t.Errorf("Given *iot.Role is not set: %+v.", builder.role)
	}
}

func TestDestinationBuilder_DeviceID(t *testing.T) {
	deviceID := "id"
	builder := &DestinationBuilder{}

	builder.DeviceID(deviceID)

	if builder.deviceID != deviceID {
		t.Errorf("Given device ID is not set: %s.", builder.deviceID)
	}
}

func TestDestinationBuilder_Build(t *testing.T) {
	deviceID := "123"
	builder := &DestinationBuilder{
		deviceID: deviceID,
	}
	destination, err := builder.Build()
	if err == nil {
		t.Fatalf("Error should be returned when *iot.Role is not set.")
	}

	role := iot.NewRole("dummy")
	builder.role = role
	destination, err = builder.Build()

	if err != nil {
		t.Fatalf("Unexpected error is returned: %s.", err.Error())
	}

	if destination.deviceID != deviceID {
		t.Errorf("Given device id is not set: %s.", destination.deviceID)
	}

	if destination.role != role {
		t.Errorf("Given *iot.Role is not set: %+v.", destination.role)
	}
}

func TestDestinationBuilder_MustBuild(t *testing.T) {
	builder := &DestinationBuilder{}
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected pahnic did not occur.")
			}
		}()
		builder.MustBuild()
	}()

	role := iot.NewRole("dummy")
	builder.role = role
	destination := builder.MustBuild()

	if destination.role != role {
		t.Errorf("Given *iot.Role is not set: %+v.", destination.role)
	}
}
