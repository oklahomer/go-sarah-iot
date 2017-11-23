package server

import (
	"errors"

	"github.com/oklahomer/go-sarah"
	"github.com/oklahomer/go-sarah-iot"
)

// Destination represents what devices to send output.
// Since not all fields are set on every occasion, this should be constructed with DestinationBuilder with proper validation.
type Destination struct {
	role     *iot.Role
	deviceID string
}

var _ sarah.OutputDestination = (*Destination)(nil)

// DestinationBuilder provides safe mechanism to build sarah.OutputDestination.
type DestinationBuilder struct {
	role     *iot.Role
	deviceID string
}

// NewDestinationBuilder returns new DestinationBuilder.
func NewDestinationBuilder() *DestinationBuilder {
	return &DestinationBuilder{}
}

// Role is a setter to designate sending destination with given role.
func (db *DestinationBuilder) Role(r *iot.Role) *DestinationBuilder {
	db.role = r
	return db
}

// DeviceID is a setter to designate sending destination with given device id.
func (db *DestinationBuilder) DeviceID(id string) *DestinationBuilder {
	db.deviceID = id
	return db
}

// Build builds Destination instance with preset arguments.
func (db *DestinationBuilder) Build() (*Destination, error) {
	if db.role == nil {
		return nil, errors.New("*iot.Role must be set")
	}

	return &Destination{
		role:     db.role,
		deviceID: db.deviceID,
	}, nil
}

type transactionalOutput struct {
	transactionID string
	c             chan []*Response
	destination   *Destination
	content       interface{}
}

var _ sarah.Output = (*transactionalOutput)(nil)

// NewTransactionalOutput creates sarah.Output that represents a request for enslaved devices and returns it with communicating channel.
func NewTransactionalOutput(transactionID string, destination *Destination, content interface{}) (sarah.Output, <-chan []*Response) {
	c := make(chan []*Response, 1)
	return &transactionalOutput{
		transactionID: transactionID,
		c:             c,
		destination:   destination,
		content:       content,
	}, c
}

func (to *transactionalOutput) Destination() sarah.OutputDestination {
	return to.destination
}

func (to *transactionalOutput) Content() interface{} {
	return to.content
}

type output struct {
	destination *Destination
	content     interface{}
}

var _ sarah.Output = (*output)(nil)

func (o *output) Destination() sarah.OutputDestination {
	return o.destination
}

func (o *output) Content() interface{} {
	return o.content
}

// NewOutput creates sarah.Output that is to be sent to given destination.
func NewOutput(destination *Destination, content interface{}) sarah.Output {
	return &output{
		destination: destination,
		content:     content,
	}
}
