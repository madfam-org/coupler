package server

import "testing"

// authMechanism must tell the truth in dry-run plans: telegram is the one
// connector that never touches janua delegation (see
// docs/CONNECTOR_TELEGRAM.md), so its plan must say so rather than echoing
// the janua_token_delegation string every OAuth connector actually uses.
func TestAuthMechanism(t *testing.T) {
	if got := authMechanism("telegram"); got != "server_static_token" {
		t.Fatalf("telegram: want server_static_token, got %q", got)
	}
	for _, connector := range []string{"github", "slack", "anything-else"} {
		if got := authMechanism(connector); got != "janua_token_delegation" {
			t.Fatalf("%s: want janua_token_delegation, got %q", connector, got)
		}
	}
}
