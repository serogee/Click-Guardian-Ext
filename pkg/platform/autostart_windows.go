//go:build windows

package platform

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	registryKey             = `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`
	registryValue           = "ClickGuardian"
	autoStartStateKey       = `SOFTWARE\ClickGuardian`
	adminAutoStartMarker    = "AdministratorAutoStart"
	adminAutoStartError     = "AdministratorAutoStartError"
	administratorTaskName   = "ClickGuardian Admin Startup"
	shellExecuteNoCloseMask = 0x00000040
	shellExecuteHide        = 0
	waitForever             = 0xffffffff
	taskQueryTimeout        = 3 * time.Second
	taskChangeTimeout       = 15 * time.Second
)

var (
	shell32            = syscall.NewLazyDLL("shell32.dll")
	procShellExecuteEx = shell32.NewProc("ShellExecuteExW")
)

type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         windows.Handle
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     windows.Handle
	lpIDList     unsafe.Pointer
	lpClass      *uint16
	hkeyClass    windows.Handle
	dwHotKey     uint32
	hIcon        windows.Handle
	hProcess     windows.Handle
}

type scheduledTask struct {
	XMLName          xml.Name             `xml:"Task"`
	Version          string               `xml:"version,attr"`
	XMLNS            string               `xml:"xmlns,attr"`
	RegistrationInfo taskRegistrationInfo `xml:"RegistrationInfo"`
	Triggers         taskTriggers         `xml:"Triggers"`
	Principals       taskPrincipals       `xml:"Principals"`
	Settings         taskSettings         `xml:"Settings"`
	Actions          taskActions          `xml:"Actions"`
}

type taskRegistrationInfo struct {
	Date        string `xml:"Date"`
	Author      string `xml:"Author"`
	Description string `xml:"Description"`
}

type taskTriggers struct {
	LogonTrigger taskLogonTrigger `xml:"LogonTrigger"`
}

type taskLogonTrigger struct {
	Enabled bool   `xml:"Enabled"`
	UserID  string `xml:"UserId"`
}

type taskPrincipals struct {
	Principal taskPrincipal `xml:"Principal"`
}

type taskPrincipal struct {
	ID        string `xml:"id,attr"`
	UserID    string `xml:"UserId"`
	LogonType string `xml:"LogonType"`
	RunLevel  string `xml:"RunLevel"`
}

type taskSettings struct {
	MultipleInstancesPolicy    string           `xml:"MultipleInstancesPolicy"`
	DisallowStartIfOnBatteries bool             `xml:"DisallowStartIfOnBatteries"`
	StopIfGoingOnBatteries     bool             `xml:"StopIfGoingOnBatteries"`
	AllowHardTerminate         bool             `xml:"AllowHardTerminate"`
	StartWhenAvailable         bool             `xml:"StartWhenAvailable"`
	RunOnlyIfNetworkAvailable  bool             `xml:"RunOnlyIfNetworkAvailable"`
	IdleSettings               taskIdleSettings `xml:"IdleSettings"`
	AllowStartOnDemand         bool             `xml:"AllowStartOnDemand"`
	Enabled                    bool             `xml:"Enabled"`
	Hidden                     bool             `xml:"Hidden"`
	RunOnlyIfIdle              bool             `xml:"RunOnlyIfIdle"`
	WakeToRun                  bool             `xml:"WakeToRun"`
	ExecutionTimeLimit         string           `xml:"ExecutionTimeLimit"`
	Priority                   int              `xml:"Priority"`
}

type taskIdleSettings struct {
	StopOnIdleEnd bool `xml:"StopOnIdleEnd"`
	RestartOnIdle bool `xml:"RestartOnIdle"`
}

type taskActions struct {
	Context string         `xml:"Context,attr"`
	Exec    taskExecAction `xml:"Exec"`
}

type taskExecAction struct {
	Command          string `xml:"Command"`
	Arguments        string `xml:"Arguments"`
	WorkingDirectory string `xml:"WorkingDirectory"`
}

type queriedTask struct {
	Settings taskSettings `xml:"Settings"`
	Actions  taskActions  `xml:"Actions"`
}

// EnableAutoStart enables standard, non-elevated startup through the current user's Run key.
func EnableAutoStart() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	autoStartCommand := fmt.Sprintf(`"%s" --minimized`, exePath)
	key, err := registry.OpenKey(registry.CURRENT_USER, registryKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open Windows startup registry key: %w", err)
	}
	defer key.Close()

	if err := key.SetStringValue(registryValue, autoStartCommand); err != nil {
		return fmt.Errorf("write Windows startup registry value: %w", err)
	}
	return nil
}

// DisableAutoStart disables the standard per-user Run entry.
func DisableAutoStart() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, registryKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open Windows startup registry key: %w", err)
	}
	defer key.Close()

	if err := key.DeleteValue(registryValue); err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("delete Windows startup registry value: %w", err)
	}
	return nil
}

// IsAutoStartEnabled reports whether standard registry startup is enabled.
func IsAutoStartEnabled() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, registryKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()

	_, _, err = key.GetStringValue(registryValue)
	return err == nil
}

// GetAutoStartMode detects elevated startup first because it takes precedence over the Run key.
func GetAutoStartMode() AutoStartMode {
	if task, exists, err := queryAdministratorTask(); err == nil && exists && isClickGuardianCommand(task.Actions.Exec.Command) {
		_ = setAdministratorMarker(true)
		return AutoStartAdministrator
	}
	if hasAdministratorMarker() {
		return AutoStartAdministrator
	}
	if IsAutoStartEnabled() {
		return AutoStartStandard
	}
	return AutoStartDisabled
}

// GetConfiguredAutoStartMode returns the locally recorded startup mode without
// starting a child process. Use it when the UI must respond immediately.
func GetConfiguredAutoStartMode() AutoStartMode {
	if hasAdministratorMarker() {
		return AutoStartAdministrator
	}
	if IsAutoStartEnabled() {
		return AutoStartStandard
	}
	return AutoStartDisabled
}

// SetAutoStartMode changes startup mode transactionally where possible.
func SetAutoStartMode(mode AutoStartMode) error {
	currentMode := GetAutoStartMode()
	standardWasEnabled := IsAutoStartEnabled()

	switch mode {
	case AutoStartAdministrator:
		// Reinstall even when already selected so legacy tasks receive --minimized and PT0S.
		if standardWasEnabled {
			if err := DisableAutoStart(); err != nil {
				return err
			}
		}
		if err := runElevatedAutoStartHelper(AdminAutoStartInstallArg); err != nil {
			if standardWasEnabled {
				_ = EnableAutoStart()
			}
			return err
		}
		return nil

	case AutoStartStandard:
		if err := EnableAutoStart(); err != nil {
			return err
		}
		if currentMode == AutoStartAdministrator || hasAdministratorMarker() {
			if err := runElevatedAutoStartHelper(AdminAutoStartRemoveArg); err != nil {
				_ = DisableAutoStart()
				return err
			}
		}
		return nil

	case AutoStartDisabled:
		if standardWasEnabled {
			if err := DisableAutoStart(); err != nil {
				return err
			}
		}
		if currentMode == AutoStartAdministrator || hasAdministratorMarker() {
			if err := runElevatedAutoStartHelper(AdminAutoStartRemoveArg); err != nil {
				if standardWasEnabled {
					_ = EnableAutoStart()
				}
				return err
			}
		}
		return nil

	default:
		return fmt.Errorf("unknown auto-start mode %d", mode)
	}
}

// InstallAdministratorAutoStart creates or migrates the app's elevated logon task.
// It is intended to be called only by the elevated helper process.
func InstallAdministratorAutoStart() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}

	if task, exists, queryErr := queryAdministratorTask(); queryErr == nil && exists {
		if !isClickGuardianCommand(task.Actions.Exec.Command) {
			return ErrAutoStartConflict
		}
	}

	account, err := user.Current()
	if err != nil {
		return fmt.Errorf("get current Windows user: %w", err)
	}

	taskXML, err := buildAdministratorTaskXML(exePath, account.Username, account.Uid)
	if err != nil {
		return err
	}

	tempFile, err := os.CreateTemp("", "click-guardian-startup-*.xml")
	if err != nil {
		return fmt.Errorf("create temporary task definition: %w", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if _, err := tempFile.Write(taskXML); err != nil {
		tempFile.Close()
		return fmt.Errorf("write temporary task definition: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temporary task definition: %w", err)
	}

	output, err := runHiddenCommand(taskChangeTimeout, "schtasks.exe", "/Create", "/TN", administratorTaskName, "/XML", tempPath, "/F")
	if err != nil {
		return fmt.Errorf("register administrator startup task: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	if err := setAdministratorMarker(true); err != nil {
		return fmt.Errorf("record administrator startup state: %w", err)
	}
	return nil
}

// RemoveAdministratorAutoStart removes only the Click Guardian elevated task.
// It is intended to be called only by the elevated helper process.
func RemoveAdministratorAutoStart() error {
	if task, exists, err := queryAdministratorTask(); err == nil && exists {
		if !isClickGuardianCommand(task.Actions.Exec.Command) {
			return ErrAutoStartConflict
		}
		output, deleteErr := runHiddenCommand(taskChangeTimeout, "schtasks.exe", "/Delete", "/TN", administratorTaskName, "/F")
		if deleteErr != nil {
			return fmt.Errorf("remove administrator startup task: %w (%s)", deleteErr, strings.TrimSpace(string(output)))
		}
	}

	if err := setAdministratorMarker(false); err != nil {
		return fmt.Errorf("clear administrator startup state: %w", err)
	}
	return nil
}

// RepairAdministratorAutoStart migrates a legacy task when this process was
// itself launched elevated by that exact task. Normal launches never elevate
// or prompt as part of this repair.
func RepairAdministratorAutoStart() error {
	if !windows.GetCurrentProcessToken().IsElevated() {
		return nil
	}

	task, exists, err := queryAdministratorTask()
	if err != nil || !exists || !isClickGuardianCommand(task.Actions.Exec.Command) {
		return nil
	}
	if err := setAdministratorMarker(true); err != nil {
		return fmt.Errorf("record administrator startup state: %w", err)
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}
	if !sameWindowsPath(task.Actions.Exec.Command, exePath) {
		return nil
	}

	if strings.TrimSpace(task.Actions.Exec.Arguments) == "--minimized" &&
		task.Settings.ExecutionTimeLimit == "PT0S" &&
		task.Settings.MultipleInstancesPolicy == "IgnoreNew" {
		return nil
	}
	return InstallAdministratorAutoStart()
}

func buildAdministratorTaskXML(exePath, username, userSID string) ([]byte, error) {
	if strings.TrimSpace(exePath) == "" || strings.TrimSpace(userSID) == "" {
		return nil, fmt.Errorf("executable path and current-user SID are required")
	}

	definition := scheduledTask{
		Version: "1.4",
		XMLNS:   "http://schemas.microsoft.com/windows/2004/02/mit/task",
		RegistrationInfo: taskRegistrationInfo{
			Date:        time.Now().Format(time.RFC3339),
			Author:      username,
			Description: "Starts Click Guardian minimized with administrator privileges at sign-in.",
		},
		Triggers: taskTriggers{LogonTrigger: taskLogonTrigger{
			Enabled: true,
			UserID:  userSID,
		}},
		Principals: taskPrincipals{Principal: taskPrincipal{
			ID:        "Author",
			UserID:    userSID,
			LogonType: "InteractiveToken",
			RunLevel:  "HighestAvailable",
		}},
		Settings: taskSettings{
			MultipleInstancesPolicy:    "IgnoreNew",
			DisallowStartIfOnBatteries: false,
			StopIfGoingOnBatteries:     false,
			AllowHardTerminate:         true,
			StartWhenAvailable:         true,
			RunOnlyIfNetworkAvailable:  false,
			IdleSettings: taskIdleSettings{
				StopOnIdleEnd: false,
				RestartOnIdle: false,
			},
			AllowStartOnDemand: true,
			Enabled:            true,
			Hidden:             false,
			RunOnlyIfIdle:      false,
			WakeToRun:          false,
			ExecutionTimeLimit: "PT0S",
			Priority:           7,
		},
		Actions: taskActions{
			Context: "Author",
			Exec: taskExecAction{
				Command:          exePath,
				Arguments:        "--minimized",
				WorkingDirectory: filepath.Dir(exePath),
			},
		},
	}

	data, err := xml.MarshalIndent(definition, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("build administrator task definition: %w", err)
	}

	// schtasks.exe imports task definitions through the Windows XML parser,
	// which expects the file bytes to match the UTF-16 declaration used by
	// Task Scheduler exports. A UTF-8 declaration can fail with "unable to
	// switch the encoding" on otherwise valid task XML.
	document := "<?xml version=\"1.0\" encoding=\"UTF-16\"?>\r\n" + string(data)
	return encodeUTF16LE(document), nil
}

func encodeUTF16LE(value string) []byte {
	units := utf16.Encode([]rune(value))
	encoded := make([]byte, 2+len(units)*2)
	encoded[0] = 0xff
	encoded[1] = 0xfe
	for index, unit := range units {
		binary.LittleEndian.PutUint16(encoded[2+index*2:], unit)
	}
	return encoded
}

func queryAdministratorTask() (*queriedTask, bool, error) {
	output, err := runHiddenCommand(taskQueryTimeout, "schtasks.exe", "/Query", "/TN", administratorTaskName, "/XML")
	if err != nil {
		return nil, false, fmt.Errorf("query administrator startup task: %w (%s)", err, strings.TrimSpace(string(output)))
	}

	task, err := parseQueriedTask(output)
	if err != nil {
		return nil, false, err
	}
	return task, true, nil
}

func runHiddenCommand(timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	command := newHiddenCommandContext(ctx, name, args...)
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		return output, fmt.Errorf("%s timed out after %s: %w", filepath.Base(name), timeout, ctx.Err())
	}
	return output, err
}

func newHiddenCommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	command := exec.CommandContext(ctx, name, args...)
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NO_WINDOW,
	}
	return command
}

func parseQueriedTask(output []byte) (*queriedTask, error) {
	var task queriedTask
	decoded := decodeTaskXMLOutput(output)
	decoder := xml.NewDecoder(bytes.NewReader(decoded))
	// schtasks may declare UTF-16 even when its redirected output has already
	// been converted to UTF-8 by the console layer.
	decoder.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) {
		return input, nil
	}
	if err := decoder.Decode(&task); err != nil {
		return nil, fmt.Errorf("parse administrator startup task: %w", err)
	}
	return &task, nil
}

func decodeTaskXMLOutput(data []byte) []byte {
	if len(data) < 2 {
		return data
	}

	var byteOrder binary.ByteOrder = binary.LittleEndian
	start := 0
	switch {
	case data[0] == 0xff && data[1] == 0xfe:
		start = 2
	case data[0] == 0xfe && data[1] == 0xff:
		byteOrder = binary.BigEndian
		start = 2
	case data[1] != 0:
		return data
	}

	units := make([]uint16, 0, (len(data)-start)/2)
	for i := start; i+1 < len(data); i += 2 {
		units = append(units, byteOrder.Uint16(data[i:i+2]))
	}
	return []byte(string(utf16.Decode(units)))
}

func isClickGuardianCommand(command string) bool {
	cleaned := strings.Trim(strings.TrimSpace(command), `"`)
	base := strings.ToLower(filepath.Base(filepath.Clean(cleaned)))
	return base == "click-guardian.exe" || base == "click-guardian-gui.exe"
}

func sameWindowsPath(first, second string) bool {
	first = strings.Trim(strings.TrimSpace(first), `"`)
	second = strings.Trim(strings.TrimSpace(second), `"`)
	firstAbs, firstErr := filepath.Abs(first)
	secondAbs, secondErr := filepath.Abs(second)
	if firstErr != nil || secondErr != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(firstAbs), filepath.Clean(secondAbs))
}

func hasAdministratorMarker() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, autoStartStateKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()

	value, _, err := key.GetIntegerValue(adminAutoStartMarker)
	return err == nil && value == 1
}

func setAdministratorMarker(enabled bool) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, autoStartStateKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()

	if enabled {
		return key.SetDWordValue(adminAutoStartMarker, 1)
	}
	if err := key.DeleteValue(adminAutoStartMarker); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}

func runElevatedAutoStartHelper(argument string) error {
	_ = recordAutoStartHelperError(nil)

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	file, err := windows.UTF16PtrFromString(exePath)
	if err != nil {
		return err
	}
	parameters, err := windows.UTF16PtrFromString(argument)
	if err != nil {
		return err
	}
	directory, err := windows.UTF16PtrFromString(filepath.Dir(exePath))
	if err != nil {
		return err
	}

	info := shellExecuteInfo{
		fMask:        shellExecuteNoCloseMask,
		lpVerb:       verb,
		lpFile:       file,
		lpParameters: parameters,
		lpDirectory:  directory,
		nShow:        shellExecuteHide,
	}
	info.cbSize = uint32(unsafe.Sizeof(info))

	result, _, callErr := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
	if result == 0 {
		if errno, ok := callErr.(syscall.Errno); ok && errno == syscall.Errno(1223) {
			return ErrElevationCancelled
		}
		return fmt.Errorf("request administrator approval: %w", callErr)
	}
	if info.hProcess == 0 {
		return fmt.Errorf("administrator helper did not return a process handle")
	}
	defer windows.CloseHandle(info.hProcess)

	if _, err := windows.WaitForSingleObject(info.hProcess, waitForever); err != nil {
		return fmt.Errorf("wait for administrator helper: %w", err)
	}

	var exitCode uint32
	if err := windows.GetExitCodeProcess(info.hProcess, &exitCode); err != nil {
		return fmt.Errorf("read administrator helper result: %w", err)
	}
	switch int(exitCode) {
	case AutoStartHelperSuccessExitCode:
		return nil
	case AutoStartHelperConflictExitCode:
		return ErrAutoStartConflict
	default:
		if message := consumeAutoStartHelperError(); message != "" {
			return fmt.Errorf("administrator startup helper: %s", message)
		}
		return fmt.Errorf("administrator startup helper failed with exit code %d", exitCode)
	}
}

// RecordAutoStartHelperError passes an elevated helper error back to the GUI process.
func RecordAutoStartHelperError(err error) {
	_ = recordAutoStartHelperError(err)
}

func recordAutoStartHelperError(helperErr error) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, autoStartStateKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()

	if helperErr != nil {
		return key.SetStringValue(adminAutoStartError, helperErr.Error())
	}
	if err := key.DeleteValue(adminAutoStartError); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}

func consumeAutoStartHelperError() string {
	key, err := registry.OpenKey(registry.CURRENT_USER, autoStartStateKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()

	message, _, err := key.GetStringValue(adminAutoStartError)
	if err != nil {
		return ""
	}
	_ = key.DeleteValue(adminAutoStartError)
	return strings.TrimSpace(message)
}
