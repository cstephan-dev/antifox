package db

import (
	"database/sql"
	"errors"
	"strings"
)

// OIDC identities link an external identity (issuer + subject) to a platform account.
// One account has at most one identity per issuer; deleting the account removes the link.

// GetOIDCUserID returns the platform user id linked to (issuer, subject), or sql.ErrNoRows.
func (d *DB) GetOIDCUserID(issuer, subject string) (string, error) {
	var id string
	err := d.QueryRow(`SELECT user_id FROM rbac_oidc_identities WHERE issuer=$1 AND subject=$2`, issuer, subject).Scan(&id)
	return id, err
}

// RBACUserHasOIDCIdentity reports whether the account is linked to any external identity.
func (d *DB) RBACUserHasOIDCIdentity(userID string) (bool, error) {
	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM rbac_oidc_identities WHERE user_id=$1`, userID).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// LinkOIDCIdentity links (issuer, subject) to userID. It fails if the account is already linked
// to another subject of the same issuer.
func (d *DB) LinkOIDCIdentity(issuer, subject, userID string) error {
	issuer, subject, userID = strings.TrimSpace(issuer), strings.TrimSpace(subject), strings.TrimSpace(userID)
	if issuer == "" || subject == "" || userID == "" {
		return errors.New("oidc identity: issuer, subject and user id are required")
	}
	var other string
	err := d.QueryRow(`SELECT subject FROM rbac_oidc_identities WHERE issuer=$1 AND user_id=$2`, issuer, userID).Scan(&other)
	switch {
	case err == nil && other != subject:
		return errors.New("oidc identity: account already linked to another identity")
	case err == nil:
		return nil
	case !errors.Is(err, sql.ErrNoRows):
		return err
	}
	_, err = d.Exec(`INSERT INTO rbac_oidc_identities(issuer,subject,user_id) VALUES ($1,$2,$3)`, issuer, subject, userID)
	return err
}
