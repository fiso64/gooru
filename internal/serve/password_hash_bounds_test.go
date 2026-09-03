package serve

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestVerifyPasswordAcceptsGooruHashContract(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := VerifyPassword(hash, "correct horse battery staple")
	if err != nil {
		t.Fatalf("verify valid hash: %v", err)
	}
	if !ok {
		t.Fatal("expected valid password to verify")
	}
}

func TestVerifyPasswordRejectsUnsupportedArgon2ParametersBeforeHashing(t *testing.T) {
	hash, err := HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"memory":      strings.Replace(hash, "m=65536", "m=4294967295", 1),
		"iterations":  strings.Replace(hash, "t=3", "t=4294967295", 1),
		"parallelism": strings.Replace(hash, "p=1", "p=4294967295", 1),
	}
	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			ok, err := VerifyPassword(encoded, "secret")
			if err == nil || !strings.Contains(err.Error(), "unsupported password hash parameters") {
				t.Fatalf("expected unsupported parameter error, got ok=%v err=%v", ok, err)
			}
		})
	}
}

func TestVerifyPasswordRejectsUnexpectedArgon2ParameterShape(t *testing.T) {
	hash, err := HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"unknown":   strings.Replace(hash, "m=65536,t=3,p=1", "m=65536,t=3,x=1", 1),
		"duplicate": strings.Replace(hash, "m=65536,t=3,p=1", "m=65536,t=3,m=65536", 1),
		"extra":     strings.Replace(hash, "m=65536,t=3,p=1", "m=65536,t=3,p=1,x=1", 1),
	}
	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			if ok, err := VerifyPassword(encoded, "secret"); err == nil || ok {
				t.Fatalf("expected invalid parameter shape to fail, got ok=%v err=%v", ok, err)
			}
		})
	}
}

func TestVerifyPasswordRejectsUnexpectedEncodedSizesBeforeDecode(t *testing.T) {
	hash, err := HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		t.Fatalf("unexpected generated hash: %q", hash)
	}

	oversizedSalt := append([]string(nil), parts...)
	oversizedSalt[4] = strings.Repeat("A", base64.RawStdEncoding.EncodedLen(passwordHashSaltBytes)+4)
	oversizedHash := append([]string(nil), parts...)
	oversizedHash[5] = strings.Repeat("A", base64.RawStdEncoding.EncodedLen(passwordHashKeyBytes)+4)

	for name, encodedParts := range map[string][]string{"salt": oversizedSalt, "hash": oversizedHash} {
		t.Run(name, func(t *testing.T) {
			ok, err := VerifyPassword(strings.Join(encodedParts, "$"), "secret")
			if err == nil || !strings.Contains(err.Error(), "unsupported password hash size") {
				t.Fatalf("expected hash size error, got ok=%v err=%v", ok, err)
			}
		})
	}
}

func TestVerifyPasswordRejectsMalformedFixedSizeBase64(t *testing.T) {
	hash, err := HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(hash, "$")

	badSalt := append([]string(nil), parts...)
	badSalt[4] = strings.Repeat("!", len(parts[4]))
	if ok, err := VerifyPassword(strings.Join(badSalt, "$"), "secret"); err == nil || ok || !strings.Contains(err.Error(), "invalid password salt") {
		t.Fatalf("expected invalid salt error, got ok=%v err=%v", ok, err)
	}

	badHash := append([]string(nil), parts...)
	badHash[5] = strings.Repeat("!", len(parts[5]))
	if ok, err := VerifyPassword(strings.Join(badHash, "$"), "secret"); err == nil || ok || !strings.Contains(err.Error(), "invalid password hash") {
		t.Fatalf("expected invalid hash error, got ok=%v err=%v", ok, err)
	}
}
