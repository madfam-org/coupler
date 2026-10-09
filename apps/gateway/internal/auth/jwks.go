package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Verifier struct {
	required bool
	issuer   string
	// audiences is the SET of accepted `aud` values, in declaration order.
	// One audience per CALLER — see acceptedAudiences.
	audiences  []string
	jwksURL    string
	httpClient *http.Client

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
}

func NewVerifier(required bool) *Verifier {
	issuer := os.Getenv("COUPLER_JANUA_ISSUER")
	if issuer == "" {
		issuer = "https://auth.madfam.io"
	}
	audiences := acceptedAudiences(os.Getenv("COUPLER_JANUA_AUDIENCE"))
	jwksURL := os.Getenv("COUPLER_JANUA_JWKS_URL")
	if jwksURL == "" {
		jwksURL = strings.TrimRight(issuer, "/") + "/.well-known/jwks.json"
	}
	return &Verifier{
		required:   required,
		issuer:     issuer,
		audiences:  audiences,
		jwksURL:    jwksURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		keys:       map[string]*rsa.PublicKey{},
	}
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

		tokenStr := strings.TrimPrefix(authz, "Bearer ")
		claims, err := v.verifyJWT(r.Context(), tokenStr)
		if err != nil {
			if !v.required && (tokenStr == "dev" || os.Getenv("COUPLER_AUTH_DEV_BYPASS") == "true") {
				claims = Claims{Sub: "dev-user", Aud: v.primaryAudience()}
			} else {
				http.Error(w, fmt.Sprintf(`{"error":"invalid_token","detail":%q}`, err.Error()), http.StatusUnauthorized)
				return
			}
		}

		ctx := context.WithValue(r.Context(), ClaimsKey, claims)
		ctx = context.WithValue(ctx, UserJWTKey, tokenStr)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

const UserJWTKey contextKey = "user_jwt"

func UserJWTFromContext(ctx context.Context) string {
	s, _ := ctx.Value(UserJWTKey).(string)
	return s
}

func (v *Verifier) verifyJWT(ctx context.Context, tokenStr string) (Claims, error) {
	var out Claims
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"RS256"}))
	token, err := parser.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		return v.getKey(ctx, kid)
	})
	if err != nil || !token.Valid {
		return out, err
	}
	mapClaims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return out, errors.New("invalid claims")
	}
	sub, _ := mapClaims["sub"].(string)
	if sub == "" {
		return out, errors.New("missing sub")
	}
	if iss, _ := mapClaims["iss"].(string); iss != "" && iss != v.issuer {
		return out, fmt.Errorf("issuer mismatch")
	}
	if !audienceOK(mapClaims["aud"], v.audiences) {
		return out, fmt.Errorf("audience mismatch")
	}
	// Carry the audience the TOKEN actually presented, not the configured set.
	// Under one-audience-per-caller that value identifies the caller, so
	// downstream handlers and audit records can tell angelia-coupler's
	// executions (coupler-api) from the gateway's own ops (coupler-gateway).
	out = Claims{Sub: sub, Aud: matchedAudience(mapClaims["aud"], v.audiences)}
	if email, _ := mapClaims["email"].(string); email != "" {
		out.Email = email
	}
	return out, nil
}

// defaultAudiences is the set of `aud` values Coupler's API accepts.
//
// ONE AUDIENCE PER CALLER (owner ruling, amended 2026-08-27). `aud` names the
// CALLER, not merely the resource, which makes it an auditable caller identity
// at the gateway door:
//
//	coupler-api      Angelia/Moirai's execution calls into Coupler, via the
//	                 angelia-coupler Janua client (angelia repo,
//	                 janua.coupler.client.yaml).
//	coupler-gateway  The gateway's own internal operations, via this repo's
//	                 janua.client.yaml.
//
// Adding a caller means registering a NEW audience and adding it here — never
// re-using an existing caller's audience. Janua reconciles client registrations
// on `audience` alone (apps/api/app/routers/v1/oauth_clients.py: registration_key
// = client_key or audience; there is no client_key column), so two clients
// sharing one audience do not get two rows: the second registration silently
// returns 200 and overwrites the first client's row.
var defaultAudiences = []string{"coupler-api", "coupler-gateway"}

// acceptedAudiences parses COUPLER_JANUA_AUDIENCE into the accepted set.
// Comma-separated for multiple; a single value stays valid, so existing
// deployments that set one audience keep working unchanged.
func acceptedAudiences(env string) []string {
	var out []string
	for _, part := range strings.Split(env, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return defaultAudiences
	}
	return out
}

// primaryAudience is the first accepted audience, used only where a single
// representative value is needed (the dev bypass, which mints no real token).
func (v *Verifier) primaryAudience() string {
	if len(v.audiences) == 0 {
		return ""
	}
	return v.audiences[0]
}

// matchedAudience returns the accepted audience the token actually presented —
// the caller's identity under one-audience-per-caller. Only called after
// audienceOK has passed; falls back to the first accepted audience for the
// no-`aud` case that audienceOK deliberately tolerates.
func matchedAudience(aud any, expected []string) string {
	first := ""
	if len(expected) > 0 {
		first = expected[0]
	}
	pick := func(s string) (string, bool) {
		for _, want := range expected {
			if s == want {
				return s, true
			}
		}
		return "", false
	}

	switch v := aud.(type) {
	case string:
		if s, ok := pick(v); ok {
			return s
		}
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				if m, matched := pick(s); matched {
					return m
				}
			}
		}
	case []string:
		for _, s := range v {
			if m, matched := pick(s); matched {
				return m
			}
		}
	}
	return first
}

// audienceOK reports whether the token's `aud` claim names one of the accepted
// callers. A token's `aud` may be a string (what Janua mints today) or an array
// (permitted by RFC 7519); either satisfies the check if any element matches
// any accepted audience.
func audienceOK(aud any, expected []string) bool {
	matches := func(s string) bool {
		for _, want := range expected {
			if s == want {
				return true
			}
		}
		return false
	}

	switch v := aud.(type) {
	case string:
		return matches(v)
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && matches(s) {
				return true
			}
		}
	case []string:
		for _, s := range v {
			if matches(s) {
				return true
			}
		}
	}
	// Preserves prior behaviour: an unconfigured audience set, or a token with
	// no `aud` at all, is not rejected on audience grounds. Issuer and
	// signature checks still apply.
	return len(expected) == 0 || aud == nil
}

func (v *Verifier) getKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	if key, ok := v.keys[kid]; ok && time.Since(v.fetchedAt) < 15*time.Minute {
		v.mu.RUnlock()
		return key, nil
	}
	v.mu.RUnlock()
	if err := v.refreshJWKS(ctx); err != nil {
		return nil, err
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	if key, ok := v.keys[kid]; ok {
		return key, nil
	}
	for _, key := range v.keys {
		return key, nil
	}
	return nil, errors.New("no jwks key")
}

func (v *Verifier) refreshJWKS(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return err
	}
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var jwks struct {
		Keys []map[string]any `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return err
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range jwks.Keys {
		kty, _ := k["kty"].(string)
		if kty != "RSA" {
			continue
		}
		kid, _ := k["kid"].(string)
		nStr, _ := k["n"].(string)
		eStr, _ := k["e"].(string)
		pub, err := rsaFromModExp(nStr, eStr)
		if err != nil {
			continue
		}
		keys[kid] = pub
	}
	v.mu.Lock()
	v.keys = keys
	v.fetchedAt = time.Now()
	v.mu.Unlock()
	return nil
}

func rsaFromModExp(nB64, eB64 string) (*rsa.PublicKey, error) {
	nb, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, err
	}
	eb, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, err
	}
	n := new(big.Int).SetBytes(nb)
	e := 0
	for _, b := range eb {
		e = e<<8 + int(b)
	}
	return &rsa.PublicKey{N: n, E: e}, nil
}
