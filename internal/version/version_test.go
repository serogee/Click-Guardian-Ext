package version

import "testing"

func TestGetVersionStringMarksDevelopmentBuilds(t *testing.T) {
	originalVersion := Version
	t.Cleanup(func() { Version = originalVersion })

	Version = "dev+abc1234"
	if got, want := GetVersionString(), "dev+abc1234 (development build)"; got != want {
		t.Fatalf("GetVersionString() = %q, want %q", got, want)
	}
}

func TestGetVersionStringLeavesReleaseVersionUnchanged(t *testing.T) {
	originalVersion := Version
	t.Cleanup(func() { Version = originalVersion })

	Version = "1.0.6"
	if got, want := GetVersionString(), "1.0.6"; got != want {
		t.Fatalf("GetVersionString() = %q, want %q", got, want)
	}
}
