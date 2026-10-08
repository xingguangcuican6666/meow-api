package oauthserver

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestValidChallengeMethod(t *testing.T) {
	// Only S256 is accepted; plain and empty are rejected (RFC 7636 §7.2).
	assert.True(t, ValidChallengeMethod(PKCEMethodS256))
	assert.False(t, ValidChallengeMethod("plain"))
	assert.False(t, ValidChallengeMethod(""))
}

func TestVerifyCodeChallenge(t *testing.T) {
	// RFC 7636 Appendix B worked example: challenge = BASE64URL(SHA256(verifier)).
	const (
		verifier  = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
		challenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	)
	assert.True(t, VerifyCodeChallenge(verifier, challenge))
	assert.False(t, VerifyCodeChallenge(verifier+"tampered", challenge))
	assert.False(t, VerifyCodeChallenge("", challenge))
	assert.False(t, VerifyCodeChallenge(verifier, ""))
}

func TestParseScopes(t *testing.T) {
	// De-duplicates on first-seen order, dropping empty/whitespace tokens.
	assert.Equal(t, []string{"openid", "profile", "email"}, ParseScopes("openid profile email"))
	assert.Equal(t, []string{"openid", "profile"}, ParseScopes("  openid   profile  openid "))
	assert.Empty(t, ParseScopes("   "))
	assert.Empty(t, ParseScopes(""))
}

func TestScopesSubsetAndSupport(t *testing.T) {
	allow := []string{"openid", "profile", "email"}
	assert.True(t, ScopesSubset([]string{"openid", "email"}, allow))
	assert.True(t, ScopesSubset(nil, allow))
	assert.False(t, ScopesSubset([]string{"openid", "groups"}, allow))

	assert.True(t, IsSupportedScope("openid"))
	assert.True(t, IsSupportedScope("groups"))
	assert.False(t, IsSupportedScope("payments"))

	assert.Equal(t, []string{"openid", "email"}, FilterSupported([]string{"openid", "payments", "email"}))
	assert.Empty(t, FilterSupported([]string{"payments"}))
}

func TestNormalizeScopeString(t *testing.T) {
	assert.Equal(t, "openid profile", NormalizeScopeString("  openid   openid profile "))
	assert.Equal(t, "", NormalizeScopeString("   "))
}

func TestClaimsForScopes(t *testing.T) {
	user := &model.User{
		Username:    "alice",
		DisplayName: "Alice Example",
		Email:       "alice@example.com",
		Role:        10,
		Group:       "default",
		Groups:      "vip,staff",
	}

	claims := ClaimsForScopes(user, []string{"profile", "email", "groups"})
	assert.Equal(t, "alice", claims["preferred_username"])
	assert.Equal(t, "Alice Example", claims["name"])
	assert.Equal(t, "alice@example.com", claims["email"])
	assert.Equal(t, true, claims["email_verified"])
	assert.Equal(t, 10, claims["role"])
	// Primary group plus extra groups, de-duplicated and sorted.
	assert.Equal(t, []string{"default", "staff", "vip"}, claims["groups"])

	// The openid scope alone produces no identity claims here (sub is added by
	// the ID-token/userinfo assembler, not by ClaimsForScopes).
	assert.Empty(t, ClaimsForScopes(user, []string{"openid"}))
	assert.Empty(t, ClaimsForScopes(user, nil))

	// DisplayName falls back to username; an empty email yields no email claim.
	minimal := &model.User{Username: "bob"}
	c2 := ClaimsForScopes(minimal, []string{"profile", "email"})
	assert.Equal(t, "bob", c2["name"])
	_, hasEmail := c2["email"]
	assert.False(t, hasEmail)
	_, hasVerified := c2["email_verified"]
	assert.False(t, hasVerified)
}

func TestMetadataDerivesEndpoints(t *testing.T) {
	const issuer = "https://id.example.com"
	meta := Metadata(issuer)
	assert.Equal(t, issuer, meta["issuer"])
	assert.Equal(t, issuer+"/oauth2/authorize", meta["authorization_endpoint"])
	assert.Equal(t, issuer+"/oauth2/token", meta["token_endpoint"])
	assert.Equal(t, issuer+"/oauth2/userinfo", meta["userinfo_endpoint"])
	assert.Equal(t, issuer+"/oauth2/jwks", meta["jwks_uri"])
	assert.Equal(t, issuer+"/oauth2/revoke", meta["revocation_endpoint"])
	assert.Equal(t, []string{"code"}, meta["response_types_supported"])
	assert.Equal(t, []string{"authorization_code", "refresh_token"}, meta["grant_types_supported"])
	assert.Equal(t, []string{PKCEMethodS256}, meta["code_challenge_methods_supported"])
	require.Contains(t, meta, "scopes_supported")
	assert.Contains(t, meta["scopes_supported"], ScopeOpenID)
}

func TestBusinessScopeCatalog(t *testing.T) {
	// wallet.topup and apikeys.manage authorize privileged actions, so they are
	// advertised as sensitive (the consent screen highlights them and only a
	// Trusted client may hold them). Neither is an OIDC identity scope.
	for _, name := range []string{ScopeWalletTopUp, ScopeAPIKeysManage} {
		scope, ok := LookupScope(name)
		require.Truef(t, ok, "%s must be in the scope catalog", name)
		assert.Truef(t, scope.Sensitive, "%s must be marked sensitive", name)
		assert.Falsef(t, scope.OIDC, "%s is not an OIDC scope", name)
	}

	// The read/invoke business scopes are non-sensitive and non-OIDC: any client
	// may request them.
	for _, name := range []string{ScopeWalletRead, ScopeModelsRead, ScopeModelsInvoke} {
		scope, ok := LookupScope(name)
		require.Truef(t, ok, "%s must be in the scope catalog", name)
		assert.Falsef(t, scope.Sensitive, "%s must not be sensitive", name)
		assert.Falsef(t, scope.OIDC, "%s is not an OIDC scope", name)
	}

	// SensitiveScopeNames is exactly the two action scopes, and ContainsSensitiveScope
	// keys off it: a grant of only read/invoke/identity scopes is not sensitive.
	assert.ElementsMatch(t, []string{ScopeWalletTopUp, ScopeAPIKeysManage}, SensitiveScopeNames())
	assert.True(t, ContainsSensitiveScope([]string{ScopeOpenID, ScopeAPIKeysManage}))
	assert.False(t, ContainsSensitiveScope([]string{ScopeOpenID, ScopeWalletRead, ScopeModelsInvoke}))

	// The removed api_keys scope must no longer resolve, and no business scope
	// contributes identity claims regardless of the user's fields.
	_, ok := LookupScope("api_keys")
	assert.False(t, ok, "the api_keys scope must be gone from the catalog")
	user := &model.User{Username: "carol", Email: "carol@example.com"}
	assert.Empty(t, ClaimsForScopes(user, []string{ScopeWalletRead, ScopeWalletTopUp, ScopeAPIKeysManage, ScopeModelsRead, ScopeModelsInvoke}))
}

func TestRefreshGrantRejectsAnotherClientsTokenWithoutConsumingIt(t *testing.T) {
	// A refresh token presented by a client it was not issued to must be rejected
	// without any write. Otherwise anyone holding a leaked refresh token could
	// destroy the victim's grant family by replaying it from their own client,
	// and the victim's next refresh would report token reuse instead of working.
	originalDB := model.DB
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.OAuthToken{}))
	model.DB = db
	t.Cleanup(func() {
		model.DB = originalDB
		if sqlDB, err := db.DB(); err == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	const victimRefresh = "rt_victim_secret"
	victim := &model.OAuthToken{
		GrantId:          "grant-victim",
		ClientId:         "client-victim",
		UserId:           7,
		Scopes:           "profile",
		AccessExpiresAt:  time.Now().Add(time.Hour).Unix(),
		RefreshExpiresAt: time.Now().Add(24 * time.Hour).Unix(),
	}
	victim.SetAccessToken("at_victim")
	victim.SetRefreshToken(victimRefresh)
	require.NoError(t, victim.Insert())

	// Another client presents the victim's refresh token.
	_, err = RefreshGrant(&model.OAuthClient{ClientId: "client-attacker"}, victimRefresh)
	require.Error(t, err)
	assert.ErrorIs(t, err, model.ErrOAuthTokenNotFound)

	// The rightful client must still be able to refresh it.
	issued, err := RefreshGrant(&model.OAuthClient{ClientId: "client-victim"}, victimRefresh)
	require.NoError(t, err)
	require.NotNil(t, issued)
	assert.NotEmpty(t, issued.RefreshToken)
}
