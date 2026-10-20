package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

// Comptes : jeton d'accès OIDC (Compte Eriphe / Authentik) dans
// Authorization: Bearer, ou ?access_token= pour le WebSocket. Facultatif : on
// joue aussi en invité ; la place d'un joueur est tenue par son jeton de place.

type User struct {
	ID   string
	Name string
}

type tokenClaims struct {
	Name              string `json:"name,omitempty"`
	PreferredUsername string `json:"preferred_username,omitempty"`
	jwt.RegisteredClaims
}

// Verifier vérifie les jetons d'accès d'un fournisseur OIDC.
type Verifier struct {
	keyfunc jwt.Keyfunc
}

// NewVerifier découvre la JWKS du fournisseur et épingle iss (et aud si donné) :
// un jeton émis par le même fournisseur pour une autre application est refusé.
func NewVerifier(ctx context.Context, issuer, audience string) (*Verifier, error) {
	discovery := strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, discovery, nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("oidc discovery: HTTP %d", res.StatusCode)
	}
	var doc struct {
		Issuer  string `json:"issuer"`
		JWKSURI string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	if doc.JWKSURI == "" {
		return nil, errors.New("oidc discovery: no jwks_uri")
	}
	kf, err := keyfunc.NewDefaultCtx(ctx, []string{doc.JWKSURI})
	if err != nil {
		return nil, fmt.Errorf("init jwks keyfunc: %w", err)
	}
	return &Verifier{keyfunc: pinClaims(kf.Keyfunc, doc.Issuer, audience)}, nil
}

func pinClaims(kf jwt.Keyfunc, issuer, audience string) jwt.Keyfunc {
	return func(t *jwt.Token) (any, error) {
		if iss, _ := t.Claims.GetIssuer(); iss != issuer {
			return nil, fmt.Errorf("unexpected issuer %q", iss)
		}
		if audience != "" {
			aud, _ := t.Claims.GetAudience()
			if !slices.Contains(aud, audience) {
				return nil, fmt.Errorf("token not issued for %q", audience)
			}
		}
		return kf(t)
	}
}

func (v *Verifier) Verify(token string) (*User, error) {
	if v == nil || v.keyfunc == nil {
		return nil, errors.New("comptes non configurés")
	}
	claims := &tokenClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, v.keyfunc)
	if err != nil {
		return nil, err
	}
	if !parsed.Valid || claims.Subject == "" {
		return nil, errors.New("jeton invalide")
	}
	name := strings.TrimSpace(claims.Name)
	if name == "" {
		name = strings.TrimSpace(claims.PreferredUsername)
	}
	return &User{ID: claims.Subject, Name: name}, nil
}

func bearer(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return r.URL.Query().Get("access_token")
}

// user renvoie le compte de la requête, nil en invité (jeton absent ou refusé).
func (s *Server) user(r *http.Request) *User {
	if s.testUser != nil {
		return s.testUser(r)
	}
	t := bearer(r)
	if t == "" || s.verifier == nil {
		return nil
	}
	u, err := s.verifier.Verify(t)
	if err != nil {
		s.log.Debug("jwt refusé", "err", err)
		return nil
	}
	return u
}
