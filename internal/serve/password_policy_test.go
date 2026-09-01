package serve

import "testing"

func TestPasswordPolicyHasNoMinimumLength(t *testing.T) {
	hash, err := HashPassword("x")
	if err != nil {
		t.Fatalf("one-character password should be accepted: %v", err)
	}
	ok, err := VerifyPassword(hash, "x")
	if err != nil || !ok {
		t.Fatalf("one-character password should verify, ok=%v err=%v", ok, err)
	}
}

func TestPasswordPolicyStillRequiresAPassword(t *testing.T) {
	if err := ValidatePassword(""); err == nil {
		t.Fatal("empty password should be rejected")
	}
}
