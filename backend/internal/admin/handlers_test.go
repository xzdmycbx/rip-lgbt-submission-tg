package admin

import "testing"

func TestValidSecretToken(t *testing.T) {
	good := []string{
		"abc",
		"ABC123",
		"a-b_c-1",
		"x", // 1 char
	}
	for _, s := range good {
		if !validSecretToken(s) {
			t.Errorf("expected %q to be accepted", s)
		}
	}

	bad := []string{
		"",
		"hello world",   // space
		"中文",            // non-ASCII
		"foo!bar",       // !
		"foo.bar",       // .
		"foo/bar",       // /
		string(make([]byte, 257, 257)), // too long
	}
	for _, s := range bad {
		if validSecretToken(s) {
			t.Errorf("expected %q to be rejected", s)
		}
	}
}

func TestValidSecretTokenMaxLength(t *testing.T) {
	at := make([]byte, 256)
	for i := range at {
		at[i] = 'a'
	}
	if !validSecretToken(string(at)) {
		t.Error("256 chars should be accepted")
	}
	over := make([]byte, 257)
	for i := range over {
		over[i] = 'a'
	}
	if validSecretToken(string(over)) {
		t.Error("257 chars should be rejected")
	}
}
