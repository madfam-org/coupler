package executor

import "testing"

func TestIntArg(t *testing.T) {
	args := map[string]any{"number": float64(42), "limit": "10"}
	if got := intArg(args, "number"); got != 42 {
		t.Fatalf("float64: got %d", got)
	}
	if got := intArg(args, "limit"); got != 10 {
		t.Fatalf("string: got %d", got)
	}
	if got := intArg(args, "missing"); got != 0 {
		t.Fatalf("missing: got %d", got)
	}
}
