package iot

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

// NewRole creates new Role instance with given value.
func NewRole(value string) *Role {
	return &Role{
		value: value,
	}
}
