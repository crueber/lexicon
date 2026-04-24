package email

import (
	"strings"
	"testing"
)

func TestEncryptDecryptPassword(t *testing.T) {
	secret := "test-secret-key-at-least-32-chars!"
	plaintext := "my-smtp-password-123"

	encrypted, err := encryptPassword(plaintext, secret)
	if err != nil {
		t.Fatalf("encryptPassword() error: %v", err)
	}

	if encrypted == "" {
		t.Fatal("encryptPassword() returned empty string")
	}
	if encrypted == plaintext {
		t.Fatal("encryptPassword() returned plaintext")
	}

	decrypted, err := decryptPassword(encrypted, secret)
	if err != nil {
		t.Fatalf("decryptPassword() error: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("decrypted = %q; want %q", decrypted, plaintext)
	}
}

func TestEncryptDecryptPassword_WrongSecret(t *testing.T) {
	secret := "test-secret-key-at-least-32-chars!"
	plaintext := "my-smtp-password-123"

	encrypted, err := encryptPassword(plaintext, secret)
	if err != nil {
		t.Fatalf("encryptPassword() error: %v", err)
	}

	_, err = decryptPassword(encrypted, "wrong-secret-key-at-least-32-chars!")
	if err == nil {
		t.Fatal("decryptPassword() with wrong secret should return error")
	}
}

func TestEncryptDecryptPassword_DifferentPlaintexts(t *testing.T) {
	secret := "test-secret-key-at-least-32-chars!"
	passwords := []string{
		"simple",
		"with special chars !@#$%",
		"unicode: 测试",
		"",
		"a-very-long-password-that-exceeds-normal-lengths-for-testing-purposes",
	}

	for _, pw := range passwords {
		encrypted, err := encryptPassword(pw, secret)
		if err != nil {
			t.Fatalf("encryptPassword(%q) error: %v", pw, err)
		}

		decrypted, err := decryptPassword(encrypted, secret)
		if err != nil {
			t.Fatalf("decryptPassword(%q) error: %v", pw, err)
		}

		if decrypted != pw {
			t.Errorf("decrypted = %q; want %q", decrypted, pw)
		}
	}
}

func TestGenerateBoundary(t *testing.T) {
	b1 := generateBoundary()
	b2 := generateBoundary()

	if b1 == "" {
		t.Error("generateBoundary() returned empty string")
	}
	if b2 == "" {
		t.Error("generateBoundary() returned empty string")
	}
	if b1 == b2 {
		t.Error("two generateBoundary() calls returned same value")
	}
	if !strings.HasPrefix(b1, "----=_Part_") {
		t.Errorf("boundary %q does not have expected prefix", b1)
	}
}
