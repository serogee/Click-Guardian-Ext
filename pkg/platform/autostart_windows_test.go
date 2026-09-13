//go:build windows

package platform

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

func TestBuildAdministratorTaskXML(t *testing.T) {
	exePath := `C:\Program Files (x86)\Click & Guardian\click-guardian.exe`
	data, err := buildAdministratorTaskXML(exePath, `MACHINE\test-user`, `S-1-5-21-1000`)
	if err != nil {
		t.Fatalf("buildAdministratorTaskXML returned an error: %v", err)
	}

	if len(data) < 2 || data[0] != 0xff || data[1] != 0xfe {
		t.Fatal("generated task XML does not have a UTF-16LE byte-order mark")
	}

	decoded := decodeTaskXMLOutput(data)
	if !bytes.Contains(decoded, []byte(`encoding="UTF-16"`)) {
		t.Fatal("generated task XML does not declare UTF-16")
	}

	var task scheduledTask
	decoder := xml.NewDecoder(bytes.NewReader(decoded))
	decoder.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) {
		return input, nil
	}
	if err := decoder.Decode(&task); err != nil {
		t.Fatalf("generated task XML is invalid: %v", err)
	}

	if task.Principals.Principal.RunLevel != "HighestAvailable" {
		t.Errorf("RunLevel = %q, want HighestAvailable", task.Principals.Principal.RunLevel)
	}
	if task.Principals.Principal.LogonType != "InteractiveToken" {
		t.Errorf("LogonType = %q, want InteractiveToken", task.Principals.Principal.LogonType)
	}
	if task.Triggers.LogonTrigger.UserID != `S-1-5-21-1000` {
		t.Errorf("logon UserId = %q", task.Triggers.LogonTrigger.UserID)
	}
	if task.Settings.ExecutionTimeLimit != "PT0S" {
		t.Errorf("ExecutionTimeLimit = %q, want PT0S", task.Settings.ExecutionTimeLimit)
	}
	if task.Settings.MultipleInstancesPolicy != "IgnoreNew" {
		t.Errorf("MultipleInstancesPolicy = %q, want IgnoreNew", task.Settings.MultipleInstancesPolicy)
	}
	if task.Actions.Exec.Command != exePath {
		t.Errorf("Command = %q, want %q", task.Actions.Exec.Command, exePath)
	}
	if task.Actions.Exec.Arguments != "--minimized" {
		t.Errorf("Arguments = %q, want --minimized", task.Actions.Exec.Arguments)
	}
	if !strings.Contains(string(decoded), "Click &amp; Guardian") {
		t.Error("generated task XML did not escape the executable path")
	}
}

func TestNewHiddenCommandContextSuppressesConsoleWindow(t *testing.T) {
	command := newHiddenCommandContext(context.Background(), "schtasks.exe", "/Query")
	if command.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil")
	}
	if !command.SysProcAttr.HideWindow {
		t.Error("HideWindow is false")
	}
	if command.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Error("CREATE_NO_WINDOW is not set")
	}
}

func TestRunHiddenCommandTimesOut(t *testing.T) {
	if os.Getenv("CLICK_GUARDIAN_TIMEOUT_HELPER") == "1" {
		time.Sleep(time.Second)
		return
	}

	t.Setenv("CLICK_GUARDIAN_TIMEOUT_HELPER", "1")
	started := time.Now()
	_, err := runHiddenCommand(50*time.Millisecond, os.Args[0], "-test.run=TestRunHiddenCommandTimesOut")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("runHiddenCommand error = %v, want context deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Errorf("runHiddenCommand returned after %s, want no more than 1s", elapsed)
	}
}

func TestParseQueriedTaskUTF16(t *testing.T) {
	source := `<?xml version="1.0" encoding="UTF-16"?><Task xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task"><Actions Context="Author"><Exec><Command>"C:\Program Files (x86)\Click Guardian\click-guardian.exe"</Command><Arguments>--minimized</Arguments></Exec></Actions></Task>`
	units := utf16.Encode([]rune(source))
	data := []byte{0xff, 0xfe}
	for _, unit := range units {
		encoded := make([]byte, 2)
		binary.LittleEndian.PutUint16(encoded, unit)
		data = append(data, encoded...)
	}

	task, err := parseQueriedTask(data)
	if err != nil {
		t.Fatalf("parseQueriedTask returned an error: %v", err)
	}
	if !isClickGuardianCommand(task.Actions.Exec.Command) {
		t.Errorf("did not recognize task command %q", task.Actions.Exec.Command)
	}
	if task.Actions.Exec.Arguments != "--minimized" {
		t.Errorf("Arguments = %q, want --minimized", task.Actions.Exec.Arguments)
	}
}

func TestIsClickGuardianCommand(t *testing.T) {
	tests := []struct {
		command string
		want    bool
	}{
		{`"C:\Program Files (x86)\Click Guardian\click-guardian.exe"`, true},
		{`C:\Tools\click-guardian-dev.exe`, true},
		// Recognize the previous GUI build name so existing startup tasks can be migrated or removed.
		{`C:\Tools\click-guardian-gui.exe`, true},
		{`C:\Tools\unrelated.exe`, false},
		{"", false},
	}

	for _, test := range tests {
		if got := isClickGuardianCommand(test.command); got != test.want {
			t.Errorf("isClickGuardianCommand(%q) = %v, want %v", test.command, got, test.want)
		}
	}
}
