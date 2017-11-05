package server

import (
	"net/http"

	"github.com/oklahomer/go-sarah-iot"
)

// Device contains authorized iot device information.
type Device struct {
	ID    string
	Roles iot.Roles
}

// Authorizer defines an interface that can be used to authorize incoming request from iot device.
//
// When requesting device is authorized, the device information is returned as *Device;
// Otherwise non-nil error is returned.
type Authorizer interface {
	Authorize(*http.Request) (*Device, error)
}
