package server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"antifox/db"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

func TestLoadOIDCConfig(t *testing.T) {
	t.Setenv("ANTIFOX_OIDC_ISSUER", "")
	if loadOIDCConfig() != nil {
		t.Fatal("no issuer: want disabled")
	}
	t.Setenv("ANTIFOX_OIDC_ISSUER", "https://sso.example.com/realms/x/")
	if loadOIDCConfig() != nil {
		t.Fatal("issuer without client id / redirect: want disabled")
	}
	t.Setenv("ANTIFOX_OIDC_CLIENT_ID", "antifox")
	t.Setenv("ANTIFOX_OIDC_REDIRECT_URL", "https://pentest.example.com/api/auth/oidc/callback")
	t.Setenv("ANTIFOX_OIDC_SCOPES", "email profile")
	t.Setenv("ANTIFOX_OIDC_ROLE_MAP", " /g/admins=admin , /g/ops=operator,bad, =x,/a=b=viewer")
	t.Setenv("ANTIFOX_OIDC_PASSWORD_LOGIN", "false")
	t.Setenv("ANTIFOX_OIDC_MAX_SESSION", "90m")
	c := loadOIDCConfig()
	if c == nil {
		t.Fatal("want enabled")
	}
	if c.Issuer != "https://sso.example.com/realms/x" {
		t.Errorf("issuer trailing slash not trimmed: %q", c.Issuer)
	}
	if strings.Join(c.Scopes, " ") != "openid email profile" {
		t.Errorf("scopes: %v (openid must be added)", c.Scopes)
	}
	want := map[string]string{"/g/admins": "admin", "/g/ops": "operator", "/a=b": "viewer"}
	if len(c.RoleMap) != len(want) {
		t.Errorf("role map: %v", c.RoleMap)
	}
	for g, r := range want {
		if c.RoleMap[g] != r {
			t.Errorf("role map %q: got %q want %q", g, c.RoleMap[g], r)
		}
	}
	if c.PasswordLogin || !c.AutoCreate || c.MaxSession != 90*time.Minute || c.UsernameClaim != "preferred_username" {
		t.Errorf("flags: %+v", c)
	}
	t.Setenv("ANTIFOX_OIDC_REDIRECT_URL", "/relative")
	if loadOIDCConfig() != nil {
		t.Fatal("relative redirect URL: want disabled")
	}
}

func TestMapOIDCRolesAndClaims(t *testing.T) {
	m := map[string]string{"/a": "admin", "/o": "operator", "/o2": "operator"}
	if got := mapOIDCRoles([]string{"/x", "/o", "/o2", "/a"}, m); strings.Join(got, ",") != "operator,admin" {
		t.Errorf("mapOIDCRoles: %v", got)
	}
	if got := mapOIDCRoles(nil, m); len(got) != 0 {
		t.Errorf("no groups: %v", got)
	}
	claims := map[string]any{"list": []any{"a", 1, "b"}, "one": "c"}
	if got := claimStrings(claims, "list"); strings.Join(got, ",") != "a,b" {
		t.Errorf("claimStrings list: %v", got)
	}
	if got := claimStrings(claims, "one"); strings.Join(got, ",") != "c" {
		t.Errorf("claimStrings string: %v", got)
	}
	if got := claimStrings(claims, "none"); got != nil {
		t.Errorf("claimStrings missing: %v", got)
	}
}

func TestOIDCCapTTL(t *testing.T) {
	s := &Server{oidc: &oidcClient{cfg: &oidcConfig{MaxSession: 8 * time.Hour}}}
	hour := time.Hour
	pw := &authClaims{}
	if got := s.oidcCapTTL(pw, hour); got != hour {
		t.Errorf("password session capped: %v", got)
	}
	fresh := &authClaims{Src: authSourceOIDC, Since: time.Now().Unix()}
	if got := s.oidcCapTTL(fresh, hour); got != hour {
		t.Errorf("fresh OIDC session: %v", got)
	}
	late := &authClaims{Src: authSourceOIDC, Since: time.Now().Add(-7*time.Hour - 30*time.Minute).Unix()}
	if got := s.oidcCapTTL(late, hour); got <= 29*time.Minute || got > 30*time.Minute {
		t.Errorf("late OIDC session: %v, want ~30m", got)
	}
	over := &authClaims{Src: authSourceOIDC, Since: time.Now().Add(-9 * time.Hour).Unix()}
	if got := s.oidcCapTTL(over, hour); got != 0 {
		t.Errorf("expired OIDC session: %v, want 0", got)
	}
	if got := s.oidcCapTTL(&authClaims{Src: authSourceOIDC}, hour); got != 0 {
		t.Errorf("OIDC session without since: %v, want 0", got)
	}
}

// fakeIdP is a minimal OIDC provider: discovery, JWKS, authorize (not called: the test reads the
// redirect) and a token endpoint that checks PKCE and returns an RS256 ID token for the next identity.
type fakeIdP struct {
	srv  *httptest.Server
	key  *rsa.PrivateKey
	mu   sync.Mutex
	next map[string]any // claims of the next ID token (sub, preferred_username, groups, …)
	// set by the test from the /start redirect; checked by /token
	nonce, challenge string
}

func newFakeIdP(t *testing.T) *fakeIdP {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIdP{key: key}
	mux := http.NewServeMux()
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	iss := f.srv.URL
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": iss, "authorization_endpoint": iss + "/auth", "token_endpoint": iss + "/token",
			"jwks_uri": iss + "/jwks", "userinfo_endpoint": iss + "/userinfo",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		b64 := base64.RawURLEncoding.EncodeToString
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{{
			"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "k1",
			"n": b64(key.N.Bytes()), "e": b64(big.NewInt(int64(key.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, _ *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"sub": f.next["sub"]})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		defer f.mu.Unlock()
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		claims := jwt.MapClaims{"iss": iss, "aud": "antifox", "nonce": f.nonce,
			"exp": time.Now().Add(5 * time.Minute).Unix(), "iat": time.Now().Unix()}
		for k, v := range f.next {
			claims[k] = v
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
		tok.Header["kid"] = "k1"
		idt, err := tok.SignedString(key)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "id_token": idt, "expires_in": 300})
	})
	return f
}

// login runs start → (IdP) → callback and returns the /login fragment parameters.
func (f *fakeIdP) login(t *testing.T, s *Server, claims map[string]any, tamperState bool) url.Values {
	t.Helper()
	w := httptest.NewRecorder()
	s.oidcStart(w, httptest.NewRequest(http.MethodGet, "/api/auth/oidc/start", nil))
	if w.Code != http.StatusFound {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}
	authURL, _ := url.Parse(w.Header().Get("Location"))
	q := authURL.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("nonce") == "" || q.Get("state") == "" {
		t.Fatalf("authorize URL lacks PKCE/nonce/state: %s", authURL)
	}
	f.mu.Lock()
	f.next, f.nonce, f.challenge = claims, q.Get("nonce"), q.Get("code_challenge")
	f.mu.Unlock()
	state := q.Get("state")
	if tamperState {
		state += "x"
	}
	r := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/callback?code=c&state="+url.QueryEscape(state), nil)
	for _, c := range w.Result().Cookies() {
		r.AddCookie(c)
	}
	w = httptest.NewRecorder()
	s.oidcCallback(w, r)
	loc := w.Header().Get("Location")
	if w.Code != http.StatusFound || !strings.HasPrefix(loc, "/login#") {
		t.Fatalf("callback: %d location=%q body=%s", w.Code, loc, w.Body.String())
	}
	v, _ := url.ParseQuery(strings.TrimPrefix(loc, "/login#"))
	return v
}

func TestOIDCLoginFlow(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	defer m.Close()
	// initialised platform: creates the built-in admin when no account exists yet
	adminHash, _ := bcrypt.GenerateFromPassword([]byte("Admin-123456789!"), bcrypt.MinCost)
	if err := m.pg.BootstrapRBAC(string(adminHash), false); err != nil {
		t.Fatal(err)
	}
	idp := newFakeIdP(t)
	key := []byte("01234567890123456789012345678901")
	cfg := &oidcConfig{Issuer: idp.srv.URL, ClientID: "antifox", RedirectURL: "https://antifox.test/api/auth/oidc/callback",
		Scopes: []string{"openid"}, UsernameClaim: "preferred_username", GroupsClaim: "groups",
		RoleMap:    map[string]string{"/af/admin": db.RBACRoleAdmin, "/af/ops": db.RBACRoleOperator, "/af/view": db.RBACRoleViewer},
		AutoCreate: true, PasswordLogin: false, MaxSession: 8 * time.Hour}
	s := &Server{m: m, jwtKey: key, oidc: newOIDCClient(cfg, key), loginLimiter: newLoginLimiter()}

	sfx := strconv.FormatInt(time.Now().UnixNano(), 10)
	name := "oidc-" + sfx
	sub := "sub-" + sfx
	cleanup := func(username string) {
		if u, err := m.pg.GetRBACUserByUsername(username); err == nil {
			_ = m.pg.DeleteRBACUser(u.ID)
		}
	}
	defer cleanup(name)
	roles := func(username string) string {
		u, err := m.pg.GetRBACUserByUsername(username)
		if err != nil {
			t.Fatalf("user %s: %v", username, err)
		}
		ids, _ := m.pg.ListRBACUserRoleIDs(u.ID)
		return strings.Join(ids, ",")
	}

	// 1. first login creates the account with the mapped role; the session token works
	v := idp.login(t, s, map[string]any{"sub": sub, "preferred_username": strings.ToUpper(name), "name": "Test User",
		"groups": []any{"/other", "/af/ops"}}, false)
	tok := v.Get("sso_token")
	if tok == "" {
		t.Fatalf("no token: %v", v)
	}
	c, ok := parseJWT(tok, key)
	if !ok || c.Src != authSourceOIDC || c.Since == 0 || c.Username != name {
		t.Fatalf("session claims: %+v", c)
	}
	if got := roles(name); got != db.RBACRoleOperator {
		t.Fatalf("roles after first login: %q", got)
	}
	h := s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	r := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("OIDC session rejected by requireAuth: %d %s", w.Code, w.Body.String())
	}

	// 2. next login with other groups: roles replaced
	idp.login(t, s, map[string]any{"sub": sub, "preferred_username": name, "groups": []any{"/af/view"}}, false)
	if got := roles(name); got != db.RBACRoleViewer {
		t.Fatalf("roles after group change: %q", got)
	}

	// 3. no mapped group: refused, roles untouched
	if v := idp.login(t, s, map[string]any{"sub": sub, "preferred_username": name, "groups": []any{"/other"}}, false); v.Get("sso_error") != "no_role" {
		t.Fatalf("unmapped groups: %v", v)
	}

	// 4. tampered state: refused
	if v := idp.login(t, s, map[string]any{"sub": sub, "preferred_username": name, "groups": []any{"/af/ops"}}, true); v.Get("sso_error") != "state" {
		t.Fatalf("tampered state: %v", v)
	}

	// 5. password login is closed for the linked account (correct password), open for the built-in admin
	u, _ := m.pg.GetRBACUserByUsername(name)
	hash, _ := bcrypt.GenerateFromPassword([]byte("Pw-123456789!"), bcrypt.MinCost)
	if err := m.pg.UpdateRBACUserPassword(u.ID, string(hash)); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	s.authLogin(w, httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"`+name+`","password":"Pw-123456789!"}`)))
	if w.Code != http.StatusForbidden {
		t.Fatalf("password login of OIDC account: %d %s", w.Code, w.Body.String())
	}
	if admin, err := m.pg.GetRBACUserByID("admin"); err == nil && admin != nil && s.oidcBlocksPasswordLogin(m.pg, admin) {
		t.Fatal("built-in admin must keep password login")
	}

	// 6. disabled account: refused
	f := false
	if err := m.pg.UpdateRBACUser(u.ID, u.DisplayName, &f, nil); err != nil {
		t.Fatal(err)
	}
	if v := idp.login(t, s, map[string]any{"sub": sub, "preferred_username": name, "groups": []any{"/af/ops"}}, false); v.Get("sso_error") != "disabled" {
		t.Fatalf("disabled account: %v", v)
	}

	// 7. an existing unlinked account with the same username is adopted (and linked)
	adopt := "oidc-adopt-" + sfx
	defer cleanup(adopt)
	if _, err := m.pg.CreateRBACUser(adopt, "", "x", true, []string{db.RBACRoleViewer}); err != nil {
		t.Fatal(err)
	}
	if v := idp.login(t, s, map[string]any{"sub": "sub-adopt-" + sfx, "preferred_username": adopt, "groups": []any{"/af/admin"}}, false); v.Get("sso_token") == "" {
		t.Fatalf("adopt existing account: %v", v)
	}
	if got := roles(adopt); got != db.RBACRoleAdmin {
		t.Fatalf("adopted account roles: %q", got)
	}
	// … and a different subject cannot take it over afterwards
	if v := idp.login(t, s, map[string]any{"sub": "sub-other-" + sfx, "preferred_username": adopt, "groups": []any{"/af/admin"}}, false); v.Get("sso_error") != "conflict" {
		t.Fatalf("second subject on linked account: %v", v)
	}

	// 8. the built-in admin is never adopted
	if v := idp.login(t, s, map[string]any{"sub": "sub-admin-" + sfx, "preferred_username": "admin", "groups": []any{"/af/admin"}}, false); v.Get("sso_error") != "conflict" {
		t.Fatalf("built-in admin adoption: %v", v)
	}
}
