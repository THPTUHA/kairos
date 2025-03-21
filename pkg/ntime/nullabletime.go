package ntime

import (
	"bytes"
	"encoding/json"
	"time"
)

type NullableTime struct {
	hasValue bool
	time     time.Time
}

func (t *NullableTime) HasValue() bool {
	return t.hasValue
}

func (t *NullableTime) Set(newTime time.Time) {
	t.hasValue = true
	t.time = newTime
}

func (t *NullableTime) Unset() {
	t.hasValue = false
}

func (t *NullableTime) Get() time.Time {
	if t.hasValue {
		return t.time
	}
	panic("runtime error: attempt to get value of NullableTime set to nil.")
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

// MarshalJSON serializes this struct to JSON
// Implements json.Marshaler interface
func (t *NullableTime) MarshalJSON() ([]byte, error) {
	if t.hasValue {
		return json.Marshal(t.time)
	}
	return json.Marshal(nil)
}

// UnmarshalJSON deserializes JSON into this struct
// Implements json.Unmarshaler interface
func (t *NullableTime) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		t.hasValue = false
		return nil
	}

	t.hasValue = true
	return json.Unmarshal(data, &t.time)
}
