package auth

import (
	"fmt"
	"net/http"
	"net/textproto"

	"github.com/oklahomer/go-sarah-iot"
)

const (
	// AuthorizerDefaultIDHeaderName is a default header name to pass device id.
	AuthorizerDefaultIDHeaderName = "X-Device-Identifier"

	// AuthorizeDefaultRolesHeaderName is a default header name to pass one or more iot.Role values.
	// When multiple values are to be passed, use multiple headers.
	AuthorizerDefaultRolesHeaderName = "X-Device-Role"

	// AuthorizerDefaultTokenHeaderName is a default header name to pass authorization token.
	AuthorizerDefaultTokenHeaderName = "X-Device-Token"
)

// Authorizer defines an interface that can be used to authorize incoming request from iot device.
//
// When requesting device is authorized, the device information is returned as *Device;
// Otherwise non-nil error is returned.
type Authorizer interface {
	Authorize(*http.Request) (*Device, error)
}

// NewAuthorizationError describes an error occurred during authorization
func NewAuthorizationError(statusCode int, message string, description string) *AuthorizationError {
	return &AuthorizationError{
		// StatusCode can be passed to http.Error to indicate error status to requester.
		StatusCode: statusCode,

		// StatusMessage can be passed to http.Error to indicate error status to requester.
		StatusMessage: message,

		// Description is a loggable representation of errror message.
		// This is not meant to be passed to external component over HTTP response.
		Description: description,
	}
}

// AuthorizationError depicts an error state that is caused by authorization failure.
type AuthorizationError struct {
	StatusCode    int
	StatusMessage string
	Description   string
}

// Error returns stringified error state.
func (e *AuthorizationError) Error() string {
	return e.StatusMessage
}

// Device contains authorized iot device information.
type Device struct {
	ID    string
	Roles iot.Roles
}

func NewDefaultAuthorizerConfig() *DefaultAuthorizerConfig {
	return &DefaultAuthorizerConfig{
		TokenValue: "",
	}
}

// DefaultAuthorizerConfig contains some configuration variables for default authorizer.
type DefaultAuthorizerConfig struct {
	TokenValue   string    `json:"token_value" yaml:"token_value"`
	AllowedRoles iot.Roles `json:"allowed_roles" yaml:"allowed_roles"`
}

// NewDefaultAuthorizer instantiates and returns new instance of default authorizer.
func NewDefaultAuthorizer(config *DefaultAuthorizerConfig) Authorizer {
	return &defaultAuthorizer{
		config: config,
	}
}

type defaultAuthorizer struct {
	config *DefaultAuthorizerConfig
}

var _ Authorizer = (*defaultAuthorizer)(nil)

func (auth *defaultAuthorizer) Authorize(req *http.Request) (*Device, error) {
	id := req.Header.Get(AuthorizerDefaultIDHeaderName)
	if id == "" {
		err := NewAuthorizationError(
			http.StatusBadRequest,
			fmt.Sprintf("%s header is not given", AuthorizerDefaultIDHeaderName),
			fmt.Sprintf("%s header is not given", AuthorizerDefaultIDHeaderName),
		)
		return nil, err
	}

	token := req.Header.Get(AuthorizerDefaultTokenHeaderName)
	if token == "" {
		err := NewAuthorizationError(
			http.StatusBadRequest,
			fmt.Sprintf("%s header is not given", AuthorizerDefaultTokenHeaderName),
			fmt.Sprintf("%s header is not given", AuthorizerDefaultTokenHeaderName),
		)
		return nil, err
	}

	if token != auth.config.TokenValue {
		err := NewAuthorizationError(
			http.StatusBadRequest,
			"Authorization Failed",
			"Unexpected token value is given.",
		)
		return nil, err
	}

	roles := iot.Roles{}
	strRoles := textproto.MIMEHeader(req.Header)[textproto.CanonicalMIMEHeaderKey(AuthorizerDefaultRolesHeaderName)]
	if len(strRoles) == 0 {
		err := NewAuthorizationError(
			http.StatusBadRequest,
			fmt.Sprintf("%s header is not given", AuthorizerDefaultRolesHeaderName),
			fmt.Sprintf("%s header is not given", AuthorizerDefaultRolesHeaderName),
		)
		return nil, err
	}

	for _, s := range strRoles {
		matched := false
		for _, r := range auth.config.AllowedRoles {
			if s == r.String() {
				roles.Append(r)
				matched = true
				break
			}
		}

		if !matched {
			// Unknown/Unacceptable role is given
			err := NewAuthorizationError(
				http.StatusBadRequest,
				fmt.Sprintf("Unauthorizable %s is given", AuthorizerDefaultRolesHeaderName),
				fmt.Sprintf("Unexpected roles are given: %v", strRoles),
			)
			return nil, err
		}
	}

	return &Device{
		ID:    id,
		Roles: roles,
	}, nil
}

// NewDebugAuthorizer instantiates and returns new instance of debug authorizer.
// This authorizer does not require requester to pass authorization token.
func NewDebugAuthorizer() Authorizer {
	return &debugAuthorizer{}
}

type debugAuthorizer struct{}

var _ Authorizer = (*debugAuthorizer)(nil)

func (auth *debugAuthorizer) Authorize(req *http.Request) (*Device, error) {
	id := req.Header.Get(AuthorizerDefaultIDHeaderName)
	if id == "" {
		id = "DebuggingDevice"
	}

	strs := textproto.MIMEHeader(req.Header)[textproto.CanonicalMIMEHeaderKey(AuthorizerDefaultRolesHeaderName)]
	if len(strs) == 0 {
		msg := fmt.Sprintf("No %s header is given.", AuthorizerDefaultRolesHeaderName)
		return nil, NewAuthorizationError(http.StatusBadRequest, msg, msg)
	}

	roles := iot.Roles{}
	for _, s := range strs {
		roles.Append(iot.NewRole(s))
	}

	return &Device{
		ID:    id,
		Roles: roles,
	}, nil
}
