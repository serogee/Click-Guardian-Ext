package platform

import "testing"

func TestAutoStartModeString(t *testing.T) {
	tests := []struct {
		mode AutoStartMode
		want string
	}{
		{AutoStartDisabled, "Disabled"},
		{AutoStartStandard, "Standard"},
		{AutoStartAdministrator, "Administrator"},
	}

	for _, test := range tests {
		if got := test.mode.String(); got != test.want {
			t.Errorf("AutoStartMode(%d).String() = %q, want %q", test.mode, got, test.want)
		}
	}
}
