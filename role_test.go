package iot

import (
	"fmt"
	"testing"
)

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

func TestRole_UnmarshalText(t *testing.T) {
	tests := []string{
		"",
		"v",
	}

	for i, test := range tests {
		i++
		role := &Role{}
		err := role.UnmarshalText([]byte(test))

		if len(test) == 0 {
			if err == nil {
				t.Errorf("Expected error is not returned on test #%d.", i)
			}
		} else {
			if role.value != test {
				t.Errorf("Expected value is not set on test #%d: %s.", i, role.value)
			}

			if err != nil {
				t.Errorf("Unexpected error is returned on test #%d: %s.", i, err.Error())
			}
		}
	}
}

func TestRole_MarshalJSON(t *testing.T) {
	value := "v"
	role := &Role{
		value: value,
	}

	b, err := role.MarshalJSON()

	if err != nil {
		t.Fatalf("Unexpected error is returned: %s.", err.Error())
	}

	if string(b) != fmt.Sprintf(`"%s"`, value) {
		t.Errorf("Unexpected value is returned: %s.", b)
	}
}

func TestRoles_Append(t *testing.T) {
	role := NewRole("v")
	roles := Roles{}

	// Append twice, but duplicated values don't count
	roles.Append(role)
	roles.Append(role)

	if len(roles) != 1 {
		t.Errorf("Unlexpected length of elements are stored: %d. %+v.", len(roles), roles)
	}
}

func TestRoles_Contains(t *testing.T) {
	stored := NewRole("stored")
	irrelevant := NewRole("irrelevant")
	roles := Roles{stored}

	if !roles.Contains(stored) {
		t.Error("Expected role is not stored.")
	}

	if roles.Contains(irrelevant) {
		t.Error("Unexpected role is stored.")
	}
}
