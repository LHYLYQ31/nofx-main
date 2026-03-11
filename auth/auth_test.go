package auth

import "testing"

func TestGenerateAndValidateJWTIncludesRole(t *testing.T) {
	SetJWTSecret("test-secret")
	token, err := GenerateJWT("u1", "u1@example.com", "ADMIN")
	if err != nil {
		t.Fatalf("generate jwt: %v", err)
	}
	claims, err := ValidateJWT(token)
	if err != nil {
		t.Fatalf("validate jwt: %v", err)
	}
	if claims.Role != "ADMIN" {
		t.Fatalf("expected role ADMIN, got %q", claims.Role)
	}
}
