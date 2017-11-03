package server

import "net/http"

type DummyAuthorizer struct {
	AuthorizeFunc func(*http.Request) (*Device, error)
}

func (d *DummyAuthorizer) Authorize(r *http.Request) (*Device, error) {
	return d.AuthorizeFunc(r)
}
