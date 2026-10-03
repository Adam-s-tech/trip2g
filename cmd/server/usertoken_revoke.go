package main

import (
	"context"

	"trip2g/internal/db"
)

// AdminRevokeUserToken revokes the token and drops it from the personal-token
// cache, so a revoked token stops resolving at once rather than when its cache
// entry expires.
func (a *app) AdminRevokeUserToken(ctx context.Context, id string) (db.UserToken, error) {
	token, err := a.WriteQueries.AdminRevokeUserToken(ctx, id)
	if err != nil {
		return token, err
	}

	a.personalTokenResolver.Forget(token.ID)

	return token, nil
}

// RevokeUserToken is the owner's own revoke; it drops the token from the
// personal-token cache for the same reason as AdminRevokeUserToken.
func (a *app) RevokeUserToken(ctx context.Context, arg db.RevokeUserTokenParams) (db.UserToken, error) {
	token, err := a.WriteQueries.RevokeUserToken(ctx, arg)
	if err != nil {
		return token, err
	}

	a.personalTokenResolver.Forget(token.ID)

	return token, nil
}
