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
	required   bool
	issuer     string
	audience   string
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
	audience := os.Getenv("COUPLER_JANUA_AUDIENCE")
	if audience == "" {
		audience = "coupler-api"
	}
	jwksURL := os.Getenv("COUPLER_JANUA_JWKS_URL")
	if jwksURL == "" {
		jwksURL = strings.TrimRight(issuer, "/") + "/.well-known/jwks.json"
	}
	return &Verifier{
		required:   required,
		issuer:     issuer,
		audience:   audience,
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
				claims = Claims{Sub: "dev-user", Aud: v.audience}
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
	if !audienceOK(mapClaims["aud"], v.audience) {
		return out, fmt.Errorf("audience mismatch")
	}
	out = Claims{Sub: sub, Aud: v.audience}
	if email, _ := mapClaims["email"].(string); email != "" {
		out.Email = email
	}
	return out, nil
}

func audienceOK(aud any, expected string) bool {
	switch v := aud.(type) {
	case string:
		return v == expected
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == expected {
				return true
			}
		}
	}
	return expected == "" || aud == nil
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
