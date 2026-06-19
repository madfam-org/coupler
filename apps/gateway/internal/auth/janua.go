package auth

import "context"

// ContextKey type for JWT claims in request context.
type contextKey string

const ClaimsKey contextKey = "janua_claims"

// Claims holds verified Janua JWT fields.
type Claims struct {
	Sub   string `json:"sub"`
	Email string `json:"email,omitempty"`
	Aud   string `json:"aud,omitempty"`
}

func ClaimsFromContext(ctx context.Context) (Claims, bool) {
	c, ok := ctx.Value(ClaimsKey).(Claims)
	return c, ok
}
