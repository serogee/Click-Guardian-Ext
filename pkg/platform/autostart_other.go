//go:build !windows

package platform

import (
	"fmt"
)

// EnableAutoStart enables the application to start with the system (not implemented for this platform)
func EnableAutoStart() error {
	return fmt.Errorf("auto-start not supported on this platform")
}

// DisableAutoStart disables the application from starting with the system (not implemented for this platform)
func DisableAutoStart() error {
	return fmt.Errorf("auto-start not supported on this platform")
}

// IsAutoStartEnabled checks if auto-start is currently enabled (not implemented for this platform)
func IsAutoStartEnabled() bool {
	return false
}

// GetAutoStartMode returns the current startup mode.
func GetAutoStartMode() AutoStartMode {
	return AutoStartDisabled
}

// SetAutoStartMode changes how the application starts with the system.
func SetAutoStartMode(mode AutoStartMode) error {
	if mode == AutoStartDisabled {
		return nil
	}
	return fmt.Errorf("auto-start not supported on this platform")
}

// InstallAdministratorAutoStart is the elevated helper operation for task creation.
func InstallAdministratorAutoStart() error {
	return fmt.Errorf("administrator auto-start not supported on this platform")
}

// RemoveAdministratorAutoStart is the elevated helper operation for task removal.
func RemoveAdministratorAutoStart() error {
	return fmt.Errorf("administrator auto-start not supported on this platform")
}

// RepairAdministratorAutoStart has no work to do outside Windows.
func RepairAdministratorAutoStart() error {
	return nil
}
