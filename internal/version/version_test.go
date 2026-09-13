package version

import "testing"

func TestGetAppInfoUsesForkBranding(t *testing.T) {
	info := GetAppInfo()
	if info.Name != "Click Guardian Ext" {
		t.Fatalf("Name = %q, want Click Guardian Ext", info.Name)
	}
	if info.Company != "Click Guardian Ext Project" {
		t.Fatalf("Company = %q, want Click Guardian Ext Project", info.Company)
	}
}

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

	Version = "1.0.0"
	if got, want := GetVersionString(), "1.0.0"; got != want {
		t.Fatalf("GetVersionString() = %q, want %q", got, want)
	}
}
