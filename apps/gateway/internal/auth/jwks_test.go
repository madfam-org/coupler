package auth

import "testing"

// ONE AUDIENCE PER CALLER (owner ruling, amended 2026-08-27).
//
// Coupler accepts a SET of audiences, each naming a distinct caller:
//   coupler-api      angelia-coupler (Moirai's execution calls into Coupler)
//   coupler-gateway  this repo's own gateway client, for internal operations
//
// The set exists because Janua reconciles client registrations on `audience`
// alone, so two callers sharing one audience collapse onto a single client row.

func TestAcceptedAudiences(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want []string
	}{
		{"unset falls back to both callers", "", defaultAudiences},
		{"single value stays valid (back-compat)", "coupler-api", []string{"coupler-api"}},
		{"comma-separated set", "coupler-api,coupler-gateway", []string{"coupler-api", "coupler-gateway"}},
		{"whitespace tolerated", " coupler-api , coupler-gateway ", []string{"coupler-api", "coupler-gateway"}},
		{"empty entries dropped", "coupler-api,,", []string{"coupler-api"}},
		{"only separators falls back", " , ", defaultAudiences},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := acceptedAudiences(tc.env)
			if len(got) != len(tc.want) {
				t.Fatalf("acceptedAudiences(%q) = %v, want %v", tc.env, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("acceptedAudiences(%q) = %v, want %v", tc.env, got, tc.want)
				}
			}
		})
	}
}

func TestAudienceOKAcceptsEitherCaller(t *testing.T) {
	set := defaultAudiences

	tests := []struct {
		name string
		aud  any
		want bool
	}{
		// The two ruled callers, as Janua mints them: a single string.
		{"angelia-coupler's execution calls", "coupler-api", true},
		{"gateway's own internal ops", "coupler-gateway", true},

		// Array form is permitted by RFC 7519 even though Janua does not mint it.
		{"array containing an accepted caller", []any{"other", "coupler-gateway"}, true},
		{"typed string slice", []string{"coupler-api"}, true},

		// Rejections.
		{"unknown caller", "some-other-service", false},
		{"array with no accepted caller", []any{"nope", "still-nope"}, false},
		{"near-miss is not a prefix match", "coupler-api-v2", false},
		{"empty string", "", false},
		{"wrong type", 42, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := audienceOK(tc.aud, set); got != tc.want {
				t.Fatalf("audienceOK(%#v, %v) = %v, want %v", tc.aud, set, got, tc.want)
			}
		})
	}
}

func TestAudienceOKPreservesUnconfiguredBehaviour(t *testing.T) {
	// Prior behaviour, read from the original implementation rather than
	// assumed: a nil `aud` fell through to the permissive tail
	// (`return expected == "" || aud == nil`) and was NOT rejected on
	// audience grounds — issuer and signature checks still apply. But a
	// REAL string `aud` against an empty expected value hit the string
	// case first (`v == expected` → `"anything" == ""` → false) and WAS
	// rejected. The set-based version keeps both: nil aud passes, and an
	// empty accepted set rejects every real audience — which is also the
	// fail-closed answer, and is unreachable in production anyway
	// (acceptedAudiences backfills defaults).
	if !audienceOK(nil, defaultAudiences) {
		t.Fatal("nil aud should not be rejected on audience grounds")
	}
	if audienceOK("anything", nil) {
		t.Fatal("an empty accepted set must reject a real audience (fail closed; matches the original string-case semantics)")
	}
}

func TestAudienceOKSingleAudienceStillIsolates(t *testing.T) {
	// A deployment pinned to one audience must NOT accept the other caller.
	single := []string{"coupler-api"}
	if !audienceOK("coupler-api", single) {
		t.Fatal("configured audience should be accepted")
	}
	if audienceOK("coupler-gateway", single) {
		t.Fatal("a single-audience deployment must not accept the other caller")
	}
}

func TestMatchedAudienceReportsCallerIdentity(t *testing.T) {
	set := defaultAudiences

	tests := []struct {
		name string
		aud  any
		want string
	}{
		{"string names the caller", "coupler-gateway", "coupler-gateway"},
		{"other caller", "coupler-api", "coupler-api"},
		{"array picks the accepted member", []any{"unrelated", "coupler-gateway"}, "coupler-gateway"},
		{"typed slice", []string{"coupler-api"}, "coupler-api"},
		// audienceOK tolerates a missing aud; matchedAudience then falls back.
		{"nil falls back to first accepted", nil, "coupler-api"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchedAudience(tc.aud, set); got != tc.want {
				t.Fatalf("matchedAudience(%#v) = %q, want %q", tc.aud, got, tc.want)
			}
		})
	}
}

func TestPrimaryAudience(t *testing.T) {
	v := &Verifier{audiences: defaultAudiences}
	if got := v.primaryAudience(); got != "coupler-api" {
		t.Fatalf("primaryAudience() = %q, want %q", got, "coupler-api")
	}
	empty := &Verifier{}
	if got := empty.primaryAudience(); got != "" {
		t.Fatalf("primaryAudience() on empty set = %q, want empty", got)
	}
}
