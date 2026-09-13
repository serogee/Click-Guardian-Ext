package platform

import (
	"errors"
	"runtime"
)

// AutoStartMode describes how the application should start with the desktop session.
type AutoStartMode int

const (
	AutoStartDisabled AutoStartMode = iota
	AutoStartStandard
	AutoStartAdministrator
)

const (
	// Auto-start helper arguments are handled before the single-instance mutex.
	AdminAutoStartInstallArg = "--install-admin-startup"
	AdminAutoStartRemoveArg  = "--remove-admin-startup"

	AutoStartHelperSuccessExitCode  = 0
	AutoStartHelperFailureExitCode  = 1
	AutoStartHelperConflictExitCode = 2
)

var (
	ErrElevationCancelled = errors.New("administrator approval was cancelled")
	ErrAutoStartConflict  = errors.New("the administrator startup task name is already used by another application")
)

func (m AutoStartMode) String() string {
	switch m {
	case AutoStartStandard:
		return "Standard"
	case AutoStartAdministrator:
		return "Administrator"
	default:
		return "Disabled"
	}
}

// Info holds platform information
type Info struct {
	OS           string
	Architecture string
	IsSupported  bool
}

// GetInfo returns information about the current platform
func GetInfo() Info {
	info := Info{
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
	}

	// Currently only Windows is fully supported
	info.IsSupported = info.OS == "windows"

	return info
}

// IsWindows returns true if running on Windows
func IsWindows() bool {
	return runtime.GOOS == "windows"
}

// IsLinux returns true if running on Linux
func IsLinux() bool {
	return runtime.GOOS == "linux"
}

// IsMacOS returns true if running on macOS
func IsMacOS() bool {
	return runtime.GOOS == "darwin"
}

// SupportedPlatforms returns a list of platforms that support mouse hooking
func SupportedPlatforms() []string {
	return []string{"windows"}
}

// PlannedPlatforms returns a list of platforms planned for future support
func PlannedPlatforms() []string {
	return []string{"linux", "darwin"}
}
