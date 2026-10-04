package pow

import (
	"encoding/hex"
	"testing"

	"golang.org/x/crypto/argon2"
)

// The shared test vector: Kafumu's Go and browser (hash-wasm) code give
// exactly this, so every OLN node agrees on the work.
func TestVector(t *testing.T) {
	h := argon2.IDKey([]byte("v2;0;20261004120000;SGVsbG8;#geo8ccgmw"), []byte("kafumu-oln-v2!!!"), 1, 4096, 1, 32)
	if got := hex.EncodeToString(h); got != "62014a657613090a939fd5a8acce60e6c720013c0cbfdf89c413337ad8873578" {
		t.Fatalf("argon2id vector = %s", got)
	}
}

func TestRoundTrip(t *testing.T) {
	line := CreatePoWMessage(4, "#geo8ccgmw #oln", "Olá ☕")
	if Work(line) < 4 {
		t.Fatalf("work %d < 4 for %s", Work(line), line)
	}
	_, _, msg, kw, err := ParsePoWMessage(line)
	if err != nil || msg != "Olá ☕" || kw != "#geo8ccgmw #oln" {
		t.Fatalf("parse = %q %q %v", msg, kw, err)
	}
	if Work("0;20261004120000;SGVsbG8;#geo8ccgmw") != 0 {
		t.Fatal("a v1 line must count as no work")
	}
}
