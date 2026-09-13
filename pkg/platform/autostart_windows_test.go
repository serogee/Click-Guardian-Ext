//go:build windows

package platform

import (
	"encoding/binary"
	"encoding/xml"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestBuildAdministratorTaskXML(t *testing.T) {
	exePath := `C:\Program Files (x86)\Click & Guardian\click-guardian.exe`
	data, err := buildAdministratorTaskXML(exePath, `MACHINE\test-user`, `S-1-5-21-1000`)
	if err != nil {
		t.Fatalf("buildAdministratorTaskXML returned an error: %v", err)
	}

	var task scheduledTask
	if err := xml.Unmarshal(data, &task); err != nil {
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
	if !strings.Contains(string(data), "Click &amp; Guardian") {
		t.Error("generated task XML did not escape the executable path")
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
