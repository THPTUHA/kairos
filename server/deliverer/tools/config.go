package tools

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

func OptionalStringChoice(v *viper.Viper, key string, choices []string) (string, error) {
	val := v.GetString(key)
	if val == "" {
		// Empty value is valid for optional configuration key.
		return val, nil
	}
	if !stringInSlice(val, choices) {
		return "", fmt.Errorf("invalid value for %s: %s, possible choices are: %s", key, val, strings.Join(choices, ", "))
	}
	return val, nil
}
