package auth

import (
	"strings"
	"testing"
)

func TestHashPasswordAndVerify(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(hash, "argon2id$v=19$m=65536,t=3,p=1$") {
		t.Fatalf("unexpected hash format: %s", hash)
	}

	ok, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected password to verify")
	}

	ok, err = VerifyPassword("wrong", hash)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("wrong password verified")
	}
}

func TestVerifyPasswordRejectsMalformedHashes(t *testing.T) {
	for _, hash := range []string{
		"",
		"argon2id$v=19$m=65536,t=3$abc$def",
		"bcrypt$v=19$m=65536,t=3,p=1$abc$def",
		"argon2id$v=16$m=65536,t=3,p=1$abc$def",
		"argon2id$v=19$m=0,t=3,p=1$abc$def",
		"argon2id$v=19$m=65536,t=3,p=1$***$def",
	} {
		if ok, err := VerifyPassword("password", hash); err == nil || ok {
			t.Fatalf("VerifyPassword(%q) = %v, %v", hash, ok, err)
		}
	}
}
