# Single sign-on (OIDC)

AntiFox can sign users in through any OpenID Connect provider (Keycloak, Microsoft Entra ID, Okta,
Authentik, Google, …). It is off by default and enabled with environment variables. Password accounts
keep working, so the built-in `admin` stays available as a break-glass account.

## How it works

1. The login page shows **Sign in with &lt;label&gt;** → `/api/auth/oidc/start` redirects to the provider
   (authorization code flow with PKCE; state and nonce in a short-lived signed cookie).
2. `/api/auth/oidc/callback` verifies the ID token, then finds the account:
   - an account already linked to this identity (issuer + `sub`), else
   - an account with the same username that is not linked yet (it gets linked; never the built-in `admin`), else
   - a new account (`ANTIFOX_OIDC_AUTO_CREATE`, default on).
3. With `ANTIFOX_OIDC_ROLE_MAP`, the account's roles are set from the user's groups on **every** login; a user
   with no mapped group is refused. Without it, roles are managed in **User Management** as usual.
4. AntiFox issues its normal session. Disabling an account or changing its password still ends its sessions at once.
   An OIDC session ends `ANTIFOX_OIDC_MAX_SESSION` (default 8h) after the OIDC login, even while in use, so the
   provider is asked again (and a user removed there loses access) at least that often.

## Configuration

| Variable | Default | |
|---|---|---|
| `ANTIFOX_OIDC_ISSUER` | — | Issuer URL; setting it enables OIDC |
| `ANTIFOX_OIDC_CLIENT_ID` | — | required |
| `ANTIFOX_OIDC_CLIENT_SECRET` | — | confidential clients; public clients rely on PKCE |
| `ANTIFOX_OIDC_REDIRECT_URL` | — | required: `https://<host>/api/auth/oidc/callback` |
| `ANTIFOX_OIDC_SCOPES` | `openid email profile` | |
| `ANTIFOX_OIDC_USERNAME_CLAIM` | `preferred_username` | lower-cased = AntiFox username |
| `ANTIFOX_OIDC_GROUPS_CLAIM` | `groups` | read from the ID token, else from userinfo |
| `ANTIFOX_OIDC_ROLE_MAP` | — | `group=role,…`; role = role id (`admin`, `operator`, `auditor`, `viewer`, or a custom role) |
| `ANTIFOX_OIDC_AUTO_CREATE` | `true` | create accounts on first login |
| `ANTIFOX_OIDC_PASSWORD_LOGIN` | `true` | `false`: accounts linked to OIDC cannot use password login (unlinked ones, like `admin`, still can) |
| `ANTIFOX_OIDC_MAX_SESSION` | `8h` | Go duration |
| `ANTIFOX_OIDC_LABEL` | `SSO` | login button text |

Example (Keycloak, groups mapper with full group path):

```bash
ANTIFOX_OIDC_ISSUER=https://sso.example.com/realms/main
ANTIFOX_OIDC_CLIENT_ID=antifox
ANTIFOX_OIDC_CLIENT_SECRET=...
ANTIFOX_OIDC_REDIRECT_URL=https://antifox.example.com/api/auth/oidc/callback
ANTIFOX_OIDC_ROLE_MAP=/antifox/admins=admin,/antifox/operators=operator,/antifox/viewers=viewer
ANTIFOX_OIDC_PASSWORD_LOGIN=false
ANTIFOX_OIDC_LABEL=Company SSO
```

Notes:
- Complete the first-run setup (admin password) before using SSO; SSO logins are refused until then.
- Behind a reverse proxy, the redirect URL must be the public URL the browser uses.
- Rate-limit `/api/auth/oidc/*` at the proxy like `/api/auth/login` if it is exposed to the internet.
