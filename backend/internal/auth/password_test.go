package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("super-secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyPassword(hash, "super-secret"); err != nil {
		t.Fatalf("verify good password: %v", err)
	}
	if err := VerifyPassword(hash, "wrong"); err == nil {
		t.Fatal("expected mismatch error for wrong password")
	}
}

func TestHashPasswordRefusesEmpty(t *testing.T) {
	if _, err := HashPassword(""); err == nil {
		t.Fatal("expected error for empty password")
	}
}

func TestVerifyPasswordRejectsBadFormats(t *testing.T) {
	cases := []string{
		"",
		"plain",
		"$bcrypt$...",
		"$argon2id$bad$$$",
	}
	for _, in := range cases {
		if err := VerifyPassword(in, "x"); err == nil {
			t.Errorf("expected error for %q", in)
		}
	}
}
