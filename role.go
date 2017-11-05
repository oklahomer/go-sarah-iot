package iot

import "fmt"

// Role represents a functional role that a device has.
//
// Each device may have one or more Roles.
// It is O.K. that multiple connections from different IoT devices share the same role.
//
// e.g. Devices located in New York and California have common role called "Thermo" that is responsible for measuring and reporting temperature.
type Role struct {
	value string
}

// String returns stringified representation of IoT role.
func (r *Role) String() string {
	return r.value
}

func (r *Role) UnmarshalText(b []byte) error {
	if len(b) == 0 {
		return fmt.Errorf("role value should not be empty")
	}

	r.value = string(b)
	return nil
}

func (r *Role) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, r.value)), nil
}

// NewRole creates new Role instance with given value.
func NewRole(value string) *Role {
	return &Role{
		value: value,
	}
}

// Roles represents set of *Roles
type Roles []*Role

// Append appends given *Role to its set.
// If one is already stored, this does nothing.
func (roles *Roles) Append(r *Role) {
	for _, role := range *roles {
		if role.String() == r.String() {
			// Already stashed
			return
		}
	}
	*roles = append(*roles, r)
}

// Contains checks if given *Role is stored in this set.
func (roles *Roles) Contains(r *Role) bool {
	for _, role := range *roles {
		if role.String() == r.String() {
			return true
		}
	}
	return false
}
