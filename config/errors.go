package config

import "fmt"

// ConfigError is a configuration error with a code and message.
type ConfigError struct {
	Code    string
	Message string
	Cause   error
}

func (e *ConfigError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *ConfigError) Unwrap() error {
	return e.Cause
}

// Wrap adds a cause to a config error.
func (e *ConfigError) Wrap(err error) *ConfigError {
	return &ConfigError{
		Code:    e.Code,
		Message: e.Message,
		Cause:   err,
	}
}

// Error codes
const (
	ErrCodePathRequired     = "CONFIG_PATH_REQUIRED"
	ErrCodeFileNotFound     = "CONFIG_FILE_NOT_FOUND"
	ErrCodeReadFailed       = "CONFIG_READ_FAILED"
	ErrCodeParseFailed      = "CONFIG_PARSE_FAILED"
	ErrCodeValidationFailed = "CONFIG_VALIDATION_FAILED"
	ErrCodeInvalidPort      = "CONFIG_INVALID_PORT"
	ErrCodeInvalidMaxPeers  = "CONFIG_INVALID_MAX_PEERS"
	ErrCodeDataDirRequired  = "CONFIG_DATA_DIR_REQUIRED"
	ErrCodeInvalidChainID   = "CONFIG_INVALID_CHAIN_ID"
)

// Predefined configuration errors
var (
	ErrConfigPathRequired = &ConfigError{
		Code:    ErrCodePathRequired,
		Message: "configuration file path is required",
	}
	ErrConfigFileNotFound = &ConfigError{
		Code:    ErrCodeFileNotFound,
		Message: "configuration file not found",
	}
	ErrConfigReadFailed = &ConfigError{
		Code:    ErrCodeReadFailed,
		Message: "failed to read configuration file",
	}
	ErrConfigParseFailed = &ConfigError{
		Code:    ErrCodeParseFailed,
		Message: "failed to parse configuration file",
	}
	ErrConfigValidationFailed = &ConfigError{
		Code:    ErrCodeValidationFailed,
		Message: "configuration validation failed",
	}
)
