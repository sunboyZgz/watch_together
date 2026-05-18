package auth

import "testing"

func TestTokenManagerIssuesAndVerifiesJWT(t *testing.T) {
	manager := NewTokenManager(DefaultTokenConfig())
	token, err := manager.IssueAccessToken(User{ID: "user_a"})
	if err != nil {
		t.Fatalf("issue access token: %v", err)
	}

	claims, err := manager.VerifyAccessToken(token)
	if err != nil {
		t.Fatalf("verify access token: %v", err)
	}
	if claims.UserID != "user_a" {
		t.Fatalf("expected user_a, got %q", claims.UserID)
	}
}

func TestTokenManagerRejectsLegacyDevToken(t *testing.T) {
	manager := NewTokenManager(DefaultTokenConfig())
	if _, err := manager.VerifyAccessToken("dev_user_a"); err == nil {
		t.Fatalf("expected legacy dev token to be rejected")
	}
}
