package iot

import "testing"

func TestNewRole(t *testing.T) {
	value := "v"
	role := NewRole(value)

	if role == nil {
		t.Fatal("Role is not returned.")
	}

	if role.value != value {
		t.Errorf("Unexpected value is set: %s.", role.value)
	}
}

func TestRole_String(t *testing.T) {
	value := "v"
	role := NewRole(value)

	if role.String() != value {
		t.Errorf("Expected role value is not returned: %s.", role.String())
	}
}
