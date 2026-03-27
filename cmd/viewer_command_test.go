package cmd

import "testing"

func TestViewerCommandIsRegistered(t *testing.T) {
	found, _, err := rootCmd.Find([]string{"viewer"})
	if err != nil {
		t.Fatalf("find viewer command: %v", err)
	}
	if found == nil {
		t.Fatalf("viewer command not found")
	}
	if found.Name() != "viewer" {
		t.Fatalf("found command name = %q, want %q", found.Name(), "viewer")
	}
}

func TestTUIAliasResolvesToViewerCommand(t *testing.T) {
	found, _, err := rootCmd.Find([]string{"tui"})
	if err != nil {
		t.Fatalf("find tui alias: %v", err)
	}
	if found == nil {
		t.Fatalf("tui alias not found")
	}
	if found.Name() != "viewer" {
		t.Fatalf("tui alias resolved to %q, want viewer", found.Name())
	}
}
