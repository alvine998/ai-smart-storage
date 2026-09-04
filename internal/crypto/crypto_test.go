package crypto

import (
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	for _, secret := range []string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "some long random passphrase"} {
		cipher, err := NewCipher(secret)
		if err != nil {
			t.Fatalf("NewCipher(%q): %v", secret, err)
		}
		sealed, err := cipher.Encrypt("halo, ini pesan rahasia")
		if err != nil {
			t.Fatalf("Encrypt: %v", sealed)
		}
		if !strings.HasPrefix(sealed, "enc:v1:") {
			t.Fatalf("sealed = %q, want enc:v1: prefix", sealed)
		}
		if sealed == "halo, ini pesan rahasia" {
			t.Fatal("content was stored as plain text")
		}
		opened, err := cipher.Decrypt(sealed)
		if err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
		if opened != "halo, ini pesan rahasia" {
			t.Fatalf("opened = %q, want original", opened)
		}
	}
}

func TestEncryptIsRandomized(t *testing.T) {
	cipher, _ := NewCipher("secret")
	first, err := cipher.Encrypt("same text")
	if err != nil {
		t.Fatal(err)
	}
	second, err := cipher.Encrypt("same text")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("expected distinct ciphertexts for the same plaintext")
	}
}

func TestEmptyKeyPassesThrough(t *testing.T) {
	cipher, err := NewCipher("   ")
	if err != nil || cipher != nil {
		t.Fatalf("NewCipher(\"\") = (%v, %v), want (nil, nil)", cipher, err)
	}
	sealed, err := cipher.Encrypt("plain")
	if err != nil || sealed != "plain" {
		t.Fatalf("Encrypt = (%q, %v), want passthrough", sealed, err)
	}
	opened, err := cipher.Decrypt("plain")
	if err != nil || opened != "plain" {
		t.Fatalf("Decrypt = (%q, %v), want passthrough", opened, err)
	}
}

func TestDecryptLegacyPlaintext(t *testing.T) {
	cipher, _ := NewCipher("secret")
	opened, err := cipher.Decrypt("row written before encryption")
	if err != nil || opened != "row written before encryption" {
		t.Fatalf("Decrypt = (%q, %v), want passthrough", opened, err)
	}
}

func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	cipher, _ := NewCipher("secret")
	sealed, err := cipher.Encrypt("secret text")
	if err != nil {
		t.Fatal(err)
	}
	tampered := sealed[:len(sealed)-2] + "xx"
	if _, err := cipher.Decrypt(tampered); err == nil {
		t.Fatal("expected error for tampered ciphertext")
	}
}

func TestDecryptFailsWithWrongKey(t *testing.T) {
	cipher, _ := NewCipher("secret")
	sealed, err := cipher.Encrypt("secret text")
	if err != nil {
		t.Fatal(err)
	}
	other, _ := NewCipher("different-secret")
	if _, err := other.Decrypt(sealed); err == nil {
		t.Fatal("expected error when decrypting with the wrong key")
	}
}

func TestEncryptEmptyStringStaysEmpty(t *testing.T) {
	cipher, _ := NewCipher("secret")
	sealed, err := cipher.Encrypt("")
	if err != nil || sealed != "" {
		t.Fatalf("Encrypt(\"\") = (%q, %v), want (\"\", nil)", sealed, err)
	}
}
