package crypto

import "testing"

func TestFieldRoundTrip(t *testing.T) {
	dek, _ := NewDEK()
	c, err := NewCipher(dek)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"", "交报告", "a:b:c"} {
		e, _ := c.Enc(s)
		if !IsCipher(e) {
			t.Fatal(e)
		}
		if s != "" && e == s {
			t.Fatal("not encrypted")
		}
		d, err := c.Dec(e)
		if err != nil || d != s {
			t.Fatalf("%q %v", d, err)
		}
	}
	if d, _ := c.Dec("明文"); d != "明文" {
		t.Fatal("plaintext passthrough")
	}
}

func TestWrap(t *testing.T) {
	dek, _ := NewDEK()
	w, err := WrapDEK(dek, "pw123")
	if err != nil {
		t.Fatal(err)
	}
	got, err := UnwrapDEK(w, "pw123")
	if err != nil || string(got) != string(dek) {
		t.Fatal(err)
	}
	if _, err := UnwrapDEK(w, "bad"); err != ErrBadPassword {
		t.Fatal(err)
	}
	rk, _ := NewRecoveryKey()
	if len(rk) != 24 {
		t.Fatal(rk)
	}
	w2, _ := WrapDEK(dek, rk)
	if _, err := UnwrapDEK(w2, NormalizeRecoveryKey(rk)); err != nil {
		t.Fatal(err)
	}
}
