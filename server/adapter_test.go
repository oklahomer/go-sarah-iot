package server

import "testing"

func TestNewConfig(t *testing.T) {
	c := NewConfig()

	if c == nil {
		t.Fatal("Pointer to Config is not returned.")
	}
}

func TestNewAdapter(t *testing.T) {
	c := &Config{}
	a, err := NewAdapter(c)

	if err != nil {
		t.Fatalf("Failed to initialize adapter: %s", err.Error())
	}

	typed, ok := a.(*adapter)
	if !ok {
		t.Fatalf("Pointer to adapter is not returned: %T", a)
	}

	if typed.config != c {
		t.Errorf("Given *Config is not set: %+v", typed.config)
	}

func TestAdapter_BotType(t *testing.T) {
	a := &adapter{}
	if a.BotType() != IoTServer {
		t.Errorf("Expected BotType is not returned: %s.", a.BotType())
	}
}
