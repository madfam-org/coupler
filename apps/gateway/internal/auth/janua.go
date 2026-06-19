package auth

import (
	"context"
	"net/http"
	"strings"
)

// ContextKey type for JWT claims in request context.
type contextKey string

const ClaimsKey contextKey = "janua_claims"

// Claims holds verified Janua JWT fields (Phase 0: optional).
type Claims struct {
	Sub   string `json:"sub"`
	Email string `json:"email,omitempty"`
	Aud   string `json:"aud,omitempty"`
}

// Verifier validates Bearer tokens. Phase 0: permissive in dev when COUPLER_AUTH_REQUIRED=false.
type Verifier struct {
	required bool
}

func NewVerifier(required bool) *Verifier {
	return &Verifier{required: required}
}

func (v *Verifier) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		authz := r.Header.Get("Authorization")
		if authz == "" {
			if v.required {
				http.Error(w, `{"error":"missing_authorization"}`, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
			return
		}

		if !strings.HasPrefix(authz, "Bearer ") {
			http.Error(w, `{"error":"invalid_authorization"}`, http.StatusUnauthorized)
			return
		}

		// Phase 1: verify RS256 via Janua JWKS
		token := strings.TrimPrefix(authz, "Bearer ")
		claims := Claims{Sub: "dev-user", Aud: "coupler-api"}
		if token != "" && token != "dev" {
			claims.Sub = "jwt-pending-verification"
		}
		ctx := context.WithValue(r.Context(), ClaimsKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func ClaimsFromContext(ctx context.Context) (Claims, bool) {
	c, ok := ctx.Value(ClaimsKey).(Claims)
	return c, ok
}
