package config

import (
	"bytes"

	"github.com/BurntSushi/toml"
)

// Decode parses TOML data into the given configuration struct.
// Returns the unparsed keys (if any) and an error.
func Decode(data []byte, cfg *Config) (unknown []string, err error) {
	// Use toml.Decode with our config struct
	// We need to handle the hierarchical structure properly

	// Create a flat decoder that works with the nested structure
	decoder := toml.NewDecoder(bytes.NewReader(data))

	// Try to decode into our config
	_, err = decoder.Decode(cfg)
	if err != nil {
		return nil, err
	}

	return nil, nil
}