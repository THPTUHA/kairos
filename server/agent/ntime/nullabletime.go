package ntime

import "time"

// NullableTime represents a Time from the time package, but can have no value set
type NullableTime struct {
	hasValue bool
	time     time.Time
}

// Set a time. This is the equivalent of an assignment
func (t *NullableTime) Set(newTime time.Time) {
	t.hasValue = true
	t.time = newTime
}

// After determines whether one time is after another.
func (t *NullableTime) After(u NullableTime) bool {
	// nil after u? No value is ever after anything else
	if !t.hasValue {
		return false
	}

	// t after nil? Always.
	if !u.hasValue {
		return true
	}

	// t after u?
	return t.time.After(u.time)
}
