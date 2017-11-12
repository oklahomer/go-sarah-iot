package auth

import (
	"net/http"
	"testing"

	"github.com/oklahomer/go-sarah-iot"
)

func TestNewDefaultAuthorizerConfig(t *testing.T) {
	config := NewDefaultAuthorizerConfig()
	if config == nil {
		t.Fatal("Nil is returned.")
	}
}

func TestAuthorizationError_Error(t *testing.T) {
	message := "error"
	e := &AuthorizationError{
		StatusMessage: message,
	}

	if e.Error() != message {
		t.Errorf("Unexpected error message is returned: %s.", e.Error())
	}
}

func TestNewAuthorizationError(t *testing.T) {
	statusCode := 403
	message := http.StatusText(statusCode)
	loggableMessage := "Loggable"

	err := NewAuthorizationError(statusCode, message, loggableMessage)

	if err.StatusCode != statusCode {
		t.Errorf("Given status code is not set: %d.", err.StatusCode)
	}

	if err.StatusMessage != message {
		t.Errorf("Given error message is not set: %s.", err.StatusMessage)
	}

	if err.Description != loggableMessage {
		t.Errorf("Given description is not set: %s.", err.Description)
	}
}

func TestNewDefaultAuthorizer(t *testing.T) {
	config := &DefaultAuthorizerConfig{}

	authorizer := NewDefaultAuthorizer(config)

	if authorizer == nil {
		t.Fatal("Authorizer is not returned.")
	}

	typed, ok := authorizer.(*defaultAuthorizer)

	if !ok {
		t.Fatalf("Instance of defaultAuthorizer is not returned: %T.", authorizer)
	}

	if typed.config != config {
		t.Errorf("Given config is not set: %+v.", typed.config)
	}
}

func TestDefaultAuthorizer_Authorize(t *testing.T) {
	expectedToken := "token"
	role1 := iot.NewRole("role1")
	role2 := iot.NewRole("role2")
	tests := []struct {
		header  http.Header
		isError bool
	}{
		{
			header:  http.Header{},
			isError: true,
		},
		{
			header: http.Header{
				AuthorizerDefaultIDHeaderName: []string{"id"},
			},
			isError: true,
		},
		{
			header: http.Header{
				AuthorizerDefaultIDHeaderName:    []string{"id"},
				AuthorizerDefaultTokenHeaderName: []string{expectedToken},
			},
			isError: true,
		},
		{
			header: http.Header{
				AuthorizerDefaultIDHeaderName:    []string{"id"},
				AuthorizerDefaultTokenHeaderName: []string{"unexpected"},
				AuthorizerDefaultRolesHeaderName: []string{role1.String(), role2.String()},
			},
			isError: true,
		},
		{
			header: http.Header{
				AuthorizerDefaultIDHeaderName:    []string{"id"},
				AuthorizerDefaultTokenHeaderName: []string{expectedToken},
				AuthorizerDefaultRolesHeaderName: []string{role1.String(), role2.String(), "irrelevant"},
			},
			isError: true,
		},
		{
			header: http.Header{
				AuthorizerDefaultIDHeaderName:    []string{"id"},
				AuthorizerDefaultTokenHeaderName: []string{expectedToken},
				AuthorizerDefaultRolesHeaderName: []string{role1.String(), role2.String()},
			},
			isError: false,
		},
	}

	config := &DefaultAuthorizerConfig{
		TokenValue:   expectedToken,
		AllowedRoles: iot.Roles{role1, role2},
	}
	authorizer := &defaultAuthorizer{config: config}
	for i, test := range tests {
		i++

		req := &http.Request{
			Header: test.header,
		}
		device, err := authorizer.Authorize(req)

		if test.isError {
			if err == nil {
				t.Errorf("Expected error is not returned on test #%d.", i)
			}
		} else {
			if device.ID != test.header.Get(AuthorizerDefaultIDHeaderName) {
				t.Errorf("Unexpected device id is returned on test #%d: %s.", i, device.ID)
			}

			for _, role := range device.Roles {
				if !config.AllowedRoles.Contains(role) {
					t.Errorf("Invalid role was approved on test #%d: %s.", i, role.String())
				}
			}
		}
	}
}

func TestNewDebugAuthorizer(t *testing.T) {
	authorizer := NewDebugAuthorizer()

	if authorizer == nil {
		t.Fatal("Authorizer is not returned.")
	}

	_, ok := authorizer.(*debugAuthorizer)

	if !ok {
		t.Fatalf("Instance of debugAuthorizer is not returned: %T.", authorizer)
	}
}

func TestDebugAuthorizer_Authorize(t *testing.T) {
	role1 := iot.NewRole("role1")
	role2 := iot.NewRole("role2")
	tests := []struct {
		header  http.Header
		isError bool
	}{
		{
			header:  http.Header{},
			isError: true,
		},
		{
			header: http.Header{
				AuthorizerDefaultIDHeaderName: []string{"id"},
			},
			isError: true,
		},
		{
			header: http.Header{
				AuthorizerDefaultIDHeaderName:    []string{"id"},
				AuthorizerDefaultRolesHeaderName: []string{role1.String(), role2.String()},
			},
			isError: false,
		},
	}

	authorizer := &debugAuthorizer{}
	for i, test := range tests {
		i++

		req := &http.Request{
			Header: test.header,
		}
		device, err := authorizer.Authorize(req)

		if test.isError {
			if err == nil {
				t.Errorf("Expected error is not returned on test #%d.", i)
			}
		} else {
			if device.ID != test.header.Get(AuthorizerDefaultIDHeaderName) {
				t.Errorf("Unexpected device id is returned on test #%d: %s.", i, device.ID)
			}

			if !(device.Roles.Contains(role1) && device.Roles.Contains(role2)) {
				t.Errorf("Expected roles are not returned on test #%d: %+v.", i, device.Roles)
			}
		}
	}
}
