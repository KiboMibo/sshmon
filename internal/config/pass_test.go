package config

import "testing"

func TestServerPass(t *testing.T) {
	if got := (Server{Password: "lit"}).Pass(); got != "lit" {
		t.Fatalf("literal: got %q, want %q", got, "lit")
	}

	t.Setenv("SSHMON_TEST_PW", "fromenv")
	if got := (Server{PasswordEnv: "SSHMON_TEST_PW"}).Pass(); got != "fromenv" {
		t.Fatalf("env: got %q, want %q", got, "fromenv")
	}

	if got := (Server{Password: "lit", PasswordEnv: "SSHMON_TEST_PW"}).Pass(); got != "lit" {
		t.Fatalf("literal precedence: got %q, want %q", got, "lit")
	}

	if got := (Server{}).Pass(); got != "" {
		t.Fatalf("unset: got %q, want empty", got)
	}

	if got := (Server{PasswordEnv: "SSHMON_TEST_MISSING"}).Pass(); got != "" {
		t.Fatalf("missing env: got %q, want empty", got)
	}
}
