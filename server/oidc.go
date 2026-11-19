package server

// OIDC single sign-on (authorization code flow + PKCE). Optional: enabled only when
// ANTIFOX_OIDC_ISSUER is set. A successful OIDC login ends in the platform's normal session
// JWT, so everything downstream (requireAuth, RBAC, token_version, disable = logout) is unchanged.
//
//	ANTIFOX_OIDC_ISSUER          issuer URL, e.g. https://sso.example.com/realms/main (enables OIDC)
//	ANTIFOX_OIDC_CLIENT_ID       client id (required)
//	ANTIFOX_OIDC_CLIENT_SECRET   client secret (optional: public clients use PKCE only)
//	ANTIFOX_OIDC_REDIRECT_URL    https://<host>/api/auth/oidc/callback (required)
//	ANTIFOX_OIDC_SCOPES          default "openid email profile"
//	ANTIFOX_OIDC_USERNAME_CLAIM  default "preferred_username" (lower-cased = platform username)
//	ANTIFOX_OIDC_GROUPS_CLAIM    default "groups"
//	ANTIFOX_OIDC_ROLE_MAP        "group=role,group2=role2" (role = platform role id, e.g. admin, operator,
//	                             auditor, viewer). When set, roles are replaced on every login and a user
//	                             without a mapped group is refused. When empty, roles are managed in the UI.
//	ANTIFOX_OIDC_AUTO_CREATE     default true: create the account on first login
//	ANTIFOX_OIDC_PASSWORD_LOGIN  default true; false = accounts linked to OIDC cannot use password login
//	                             (accounts without a link, e.g. the built-in admin, still can)
//	ANTIFOX_OIDC_MAX_SESSION     default 8h: an OIDC session ends this long after the OIDC login, even
//	                             while in use (the identity provider is then asked again)
//	ANTIFOX_OIDC_LABEL           login button text, default "SSO"

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"antifox/db"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
)

const (
	oidcStateCookie = "antifox_oidc"
	oidcStatePath   = "/api/auth/oidc/"
	oidcStateTTL    = 10 * time.Minute
	authSourceOIDC  = "oidc"
)

type oidcConfig struct {
	Issuer, ClientID, ClientSecret, RedirectURL, Label string
	Scopes                                             []string
	UsernameClaim, GroupsClaim                         string
	RoleMap                                            map[string]string // group -> role id
	AutoCreate, PasswordLogin                          bool
	MaxSession                                         time.Duration
}

// loadOIDCConfig reads the environment. nil = OIDC disabled (not configured or misconfigured).
func loadOIDCConfig() *oidcConfig {
	env := func(k, def string) string {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
		return def
	}
	boolEnv := func(k string, def bool) bool {
		if b, err := strconv.ParseBool(env(k, "")); err == nil {
			return b
		}
		return def
	}
	c := &oidcConfig{
		Issuer:        strings.TrimRight(env("ANTIFOX_OIDC_ISSUER", ""), "/"),
		ClientID:      env("ANTIFOX_OIDC_CLIENT_ID", ""),
		ClientSecret:  env("ANTIFOX_OIDC_CLIENT_SECRET", ""),
		RedirectURL:   env("ANTIFOX_OIDC_REDIRECT_URL", ""),
		Label:         env("ANTIFOX_OIDC_LABEL", "SSO"),
		Scopes:        strings.Fields(env("ANTIFOX_OIDC_SCOPES", "openid email profile")),
		UsernameClaim: env("ANTIFOX_OIDC_USERNAME_CLAIM", "preferred_username"),
		GroupsClaim:   env("ANTIFOX_OIDC_GROUPS_CLAIM", "groups"),
		RoleMap:       map[string]string{},
		AutoCreate:    boolEnv("ANTIFOX_OIDC_AUTO_CREATE", true),
		PasswordLogin: boolEnv("ANTIFOX_OIDC_PASSWORD_LOGIN", true),
		MaxSession:    8 * time.Hour,
	}
	if c.Issuer == "" {
		return nil
	}
	if c.ClientID == "" || c.RedirectURL == "" {
		log.Printf("[oidc] disabled: ANTIFOX_OIDC_CLIENT_ID and ANTIFOX_OIDC_REDIRECT_URL are required")
		return nil
	}
	if u, err := url.Parse(c.RedirectURL); err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		log.Printf("[oidc] disabled: ANTIFOX_OIDC_REDIRECT_URL is not an absolute http(s) URL")
		return nil
	}
	if v := env("ANTIFOX_OIDC_MAX_SESSION", ""); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			c.MaxSession = d
		} else {
			log.Printf("[oidc] ANTIFOX_OIDC_MAX_SESSION=%q is not a duration, using %s", v, c.MaxSession)
		}
	}
	for _, pair := range strings.Split(env("ANTIFOX_OIDC_ROLE_MAP", ""), ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		i := strings.LastIndex(pair, "=")
		if i <= 0 || i == len(pair)-1 {
			log.Printf("[oidc] ignoring ANTIFOX_OIDC_ROLE_MAP entry %q (want group=role)", pair)
			continue
		}
		c.RoleMap[strings.TrimSpace(pair[:i])] = strings.TrimSpace(pair[i+1:])
	}
	if !containsStr(c.Scopes, "openid") {
		c.Scopes = append([]string{"openid"}, c.Scopes...)
	}
	return c
}

func containsStr(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// oidcClient holds the config and the lazily discovered provider (discovery is retried until it
// succeeds, so a provider that is down at startup does not disable SSO for the process lifetime).
type oidcClient struct {
	cfg      *oidcConfig
	stateKey []byte

	mu       sync.Mutex
	provider *oidc.Provider
}

func newOIDCClient(cfg *oidcConfig, jwtKey []byte) *oidcClient {
	if cfg == nil {
		return nil
	}
	// separate key for the state cookie: it must never be accepted as a session token
	m := hmac.New(sha256.New, jwtKey)
	m.Write([]byte("antifox-oidc-state"))
	log.Printf("[oidc] enabled: issuer=%s client=%s role_map=%d groups auto_create=%t password_login=%t max_session=%s",
		cfg.Issuer, cfg.ClientID, len(cfg.RoleMap), cfg.AutoCreate, cfg.PasswordLogin, cfg.MaxSession)
	return &oidcClient{cfg: cfg, stateKey: m.Sum(nil)}
}

func (o *oidcClient) oauth(ctx context.Context) (*oauth2.Config, *oidc.Provider, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.provider == nil {
		dctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		p, err := oidc.NewProvider(dctx, o.cfg.Issuer)
		if err != nil {
			return nil, nil, fmt.Errorf("discovery: %w", err)
		}
		o.provider = p
	}
	return &oauth2.Config{
		ClientID:     o.cfg.ClientID,
		ClientSecret: o.cfg.ClientSecret,
		RedirectURL:  o.cfg.RedirectURL,
		Endpoint:     o.provider.Endpoint(),
		Scopes:       o.cfg.Scopes,
	}, o.provider, nil
}

type oidcStateClaims struct {
	State    string `json:"st"`
	Nonce    string `json:"nn"`
	Verifier string `json:"pv"`
	jwt.RegisteredClaims
}

func randomToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// oidcFail ends the browser flow on the login page with an error code (shown translated there).
func oidcFail(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, "/login#sso_error="+url.QueryEscape(code), http.StatusFound)
}

func (o *oidcClient) setStateCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: oidcStateCookie, Value: value, Path: oidcStatePath, MaxAge: maxAge,
		HttpOnly: true, Secure: strings.HasPrefix(o.cfg.RedirectURL, "https://"), SameSite: http.SameSiteLaxMode,
	})
}

// GET /api/auth/oidc/start — redirect the browser to the identity provider.
func (s *Server) oidcStart(w http.ResponseWriter, r *http.Request) {
	o := s.oidc
	if o == nil {
		writeErr(w, 404, "OIDC is not enabled")
		return
	}
	oc, _, err := o.oauth(r.Context())
	if err != nil {
		log.Printf("[oidc] %v", err)
		oidcFail(w, r, "unavailable")
		return
	}
	st := oidcStateClaims{State: randomToken(), Nonce: randomToken(), Verifier: oauth2.GenerateVerifier(),
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(oidcStateTTL))}}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, st).SignedString(o.stateKey)
	if err != nil {
		oidcFail(w, r, "failed")
		return
	}
	o.setStateCookie(w, signed, int(oidcStateTTL.Seconds()))
	http.Redirect(w, r, oc.AuthCodeURL(st.State, oidc.Nonce(st.Nonce), oauth2.S256ChallengeOption(st.Verifier)), http.StatusFound)
}

// GET /api/auth/oidc/callback — finish the code flow, map the identity to an account, issue a session.
func (s *Server) oidcCallback(w http.ResponseWriter, r *http.Request) {
	o := s.oidc
	if o == nil {
		writeErr(w, 404, "OIDC is not enabled")
		return
	}
	pg := s.pg(w)
	if pg == nil {
		return
	}
	ip := clientIP(r)
	fail := func(code, reason, actor string) {
		pg.RecordAudit(actor, "auth", "oidc_login", "fail", ip, reason, nil)
		log.Printf("[oidc] login refused user=%q ip=%s: %s", actor, ip, reason)
		oidcFail(w, r, code)
	}

	// state cookie: one use only
	c, err := r.Cookie(oidcStateCookie)
	o.setStateCookie(w, "", -1)
	if err != nil {
		fail("state", "missing state cookie (expired or opened in another browser)", "")
		return
	}
	var st oidcStateClaims
	tok, err := jwt.ParseWithClaims(c.Value, &st, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return o.stateKey, nil
	})
	if err != nil || !tok.Valid || st.State == "" || !hmac.Equal([]byte(st.State), []byte(r.URL.Query().Get("state"))) {
		fail("state", "invalid or mismatched state", "")
		return
	}
	if e := r.URL.Query().Get("error"); e != "" {
		fail("idp", "identity provider returned error: "+e, "")
		return
	}

	oc, provider, err := o.oauth(r.Context())
	if err != nil {
		log.Printf("[oidc] %v", err)
		oidcFail(w, r, "unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	t, err := oc.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(st.Verifier))
	if err != nil {
		fail("failed", "code exchange failed: "+err.Error(), "")
		return
	}
	raw, _ := t.Extra("id_token").(string)
	if raw == "" {
		fail("failed", "no id_token in token response", "")
		return
	}
	idt, err := provider.Verifier(&oidc.Config{ClientID: o.cfg.ClientID}).Verify(ctx, raw)
	if err != nil {
		fail("failed", "id_token verification failed: "+err.Error(), "")
		return
	}
	if !hmac.Equal([]byte(idt.Nonce), []byte(st.Nonce)) {
		fail("failed", "id_token nonce mismatch", "")
		return
	}
	var claims map[string]any
	if err := idt.Claims(&claims); err != nil {
		fail("failed", "cannot read id_token claims", "")
		return
	}
	// Prefer the userinfo endpoint for groups/name when the ID token lacks them (some providers
	// only put groups there); its claims never override the ID token's.
	if _, ok := claims[o.cfg.GroupsClaim]; !ok {
		if ui, err := provider.UserInfo(ctx, oauth2.StaticTokenSource(t)); err == nil {
			var extra map[string]any
			if ui.Subject == idt.Subject && ui.Claims(&extra) == nil {
				for k, v := range extra {
					if _, exists := claims[k]; !exists {
						claims[k] = v
					}
				}
			}
		}
	}

	username := strings.ToLower(strings.TrimSpace(claimString(claims, o.cfg.UsernameClaim)))
	if username == "" {
		fail("failed", "id_token has no "+o.cfg.UsernameClaim+" claim", "")
		return
	}
	displayName := strings.TrimSpace(claimString(claims, "name"))

	var roles []string
	if len(o.cfg.RoleMap) > 0 {
		roles = mapOIDCRoles(claimStrings(claims, o.cfg.GroupsClaim), o.cfg.RoleMap)
		if len(roles) == 0 {
			fail("no_role", "no group of the user is mapped to a role", username)
			return
		}
	}

	u, err := s.oidcResolveUser(pg, o.cfg, idt.Subject, username, displayName, roles)
	if err != nil {
		var le oidcLoginError
		if errors.As(err, &le) {
			fail(le.code, le.reason, username)
		} else {
			log.Printf("[oidc] account lookup failed: %v", err)
			fail("failed", "account lookup failed", username)
		}
		return
	}
	if !u.Enabled {
		fail("disabled", "account is disabled", u.Username)
		return
	}
	if roles != nil {
		if err := pg.UpdateRBACUser(u.ID, firstNonEmpty(displayName, u.DisplayName), nil, &roles); err != nil {
			log.Printf("[oidc] role update failed for %q: %v", u.Username, err)
			fail("failed", "role update failed", u.Username)
			return
		}
	}
	sc := authClaims{Username: u.Username, Ver: u.TokenVersion, Src: authSourceOIDC, Since: time.Now().Unix(),
		RegisteredClaims: jwt.RegisteredClaims{Subject: u.ID}}
	session, err := signSessionJWT(s.jwtKey, sc, s.oidcCapTTL(&sc, s.sessionTTL()))
	if err != nil {
		fail("failed", "token generation failed", u.Username)
		return
	}
	pg.RecordAudit(u.Username, "auth", "login", "success", ip, "OIDC login ("+strings.Join(roles, ",")+")", nil)
	log.Printf("[oidc] login user=%q ip=%s roles=%v", u.Username, ip, roles)
	// the token goes in the fragment: never sent to a server, not in access logs or Referer
	http.Redirect(w, r, "/login#sso_token="+url.QueryEscape(session), http.StatusFound)
}

type oidcLoginError struct{ code, reason string }

func (e oidcLoginError) Error() string { return e.reason }

// oidcResolveUser finds the account for (issuer, subject): an existing link, else an account with the
// same username that has no link yet and is not built-in (adopted and linked), else a new account.
func (s *Server) oidcResolveUser(pg *db.DB, cfg *oidcConfig, subject, username, displayName string, roles []string) (*db.RBACUser, error) {
	// the built-in admin's username is reserved (also before first-run setup created it), and SSO waits
	// for setup: an account created by SSO would otherwise mark the platform initialised
	if username == "admin" {
		return nil, oidcLoginError{"conflict", "username belongs to a built-in account"}
	}
	if needs, err := pg.RBACNeedsBootstrap(); err != nil {
		return nil, err
	} else if needs {
		return nil, oidcLoginError{"not_initialized", "platform setup has not been completed"}
	}
	if id, err := pg.GetOIDCUserID(cfg.Issuer, subject); err == nil {
		return pg.GetRBACUserByID(id)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	u, err := pg.GetRBACUserByUsername(username)
	switch {
	case err == nil:
		if u.IsBuiltin {
			return nil, oidcLoginError{"conflict", "username belongs to a built-in account"}
		}
		if err := pg.LinkOIDCIdentity(cfg.Issuer, subject, u.ID); err != nil {
			return nil, oidcLoginError{"conflict", "account already linked to another identity"}
		}
		log.Printf("[oidc] linked existing account %q to subject %s", u.Username, subject)
		return u, nil
	case !errors.Is(err, sql.ErrNoRows):
		return nil, err
	}
	if !cfg.AutoCreate {
		return nil, oidcLoginError{"no_account", "no account for this user and auto-create is off"}
	}
	// unusable random password: the account signs in through OIDC only (until an admin sets one)
	hash, err := bcrypt.GenerateFromPassword([]byte(randomToken()), bcrypt.DefaultCost) // 48 hex chars (bcrypt max 72 bytes)
	if err != nil {
		return nil, err
	}
	u, err = pg.CreateRBACUser(username, displayName, string(hash), true, roles)
	if err != nil {
		return nil, err
	}
	if err := pg.LinkOIDCIdentity(cfg.Issuer, subject, u.ID); err != nil {
		return nil, err
	}
	log.Printf("[oidc] created account %q (subject %s)", u.Username, subject)
	return u, nil
}

// mapOIDCRoles returns the distinct role ids of the groups found in the map, in map-hit order.
func mapOIDCRoles(groups []string, roleMap map[string]string) []string {
	roles := []string{}
	for _, g := range groups {
		if r, ok := roleMap[g]; ok && !containsStr(roles, r) {
			roles = append(roles, r)
		}
	}
	return roles
}

func claimString(claims map[string]any, key string) string {
	s, _ := claims[key].(string)
	return s
}

// claimStrings reads a claim that is a list of strings (or a single string).
func claimStrings(claims map[string]any, key string) []string {
	switch v := claims[key].(type) {
	case string:
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// oidcStatus is the public part of the configuration, for the login page.
func (s *Server) oidcStatus() map[string]any {
	if s.oidc == nil {
		return map[string]any{"enabled": false}
	}
	return map[string]any{"enabled": true, "label": s.oidc.cfg.Label, "password_login": s.oidc.cfg.PasswordLogin}
}

// oidcBlocksPasswordLogin reports whether u must sign in through OIDC (ANTIFOX_OIDC_PASSWORD_LOGIN=false
// and the account is linked to an identity). Fails closed on a lookup error.
func (s *Server) oidcBlocksPasswordLogin(pg *db.DB, u *db.RBACUser) bool {
	if s.oidc == nil || s.oidc.cfg.PasswordLogin || u == nil {
		return false
	}
	linked, err := pg.RBACUserHasOIDCIdentity(u.ID)
	return err != nil || linked
}

// oidcCapTTL caps a session token's lifetime so an OIDC session ends ANTIFOX_OIDC_MAX_SESSION after
// the OIDC login, however active it is (the identity provider is then asked again). 0 = do not
// issue/refresh. Password sessions are returned unchanged.
func (s *Server) oidcCapTTL(c *authClaims, ttl time.Duration) time.Duration {
	if c.Src != authSourceOIDC || s.oidc == nil {
		return ttl
	}
	if c.Since <= 0 {
		return 0
	}
	if left := time.Until(time.Unix(c.Since, 0).Add(s.oidc.cfg.MaxSession)); left < ttl {
		if left <= 0 {
			return 0
		}
		return left
	}
	return ttl
}
