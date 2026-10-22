package security

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	password := "Local-Development-Password-1"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if !VerifyPassword(hash, password) {
		t.Fatal("VerifyPassword() rejected the matching password")
	}
	if VerifyPassword(hash, "a different password") {
		t.Fatal("VerifyPassword() accepted a different password")
	}
}

func TestHashPasswordRejectsOutsideBounds(t *testing.T) {
	for _, password := range []string{"short", string(make([]byte, MaxPasswordBytes+1))} {
		if _, err := HashPassword(password); err == nil {
			t.Errorf("HashPassword(%d bytes) accepted an invalid password", len(password))
		}
	}
}
