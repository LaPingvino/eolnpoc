package main

import (
	"testing"
	"time"

	"github.com/lapingvino/eolnpoc/olnjson"
	"github.com/lapingvino/eolnpoc/pow"
)

func TestTrustworthy(t *testing.T) {
	now := time.Now().UTC()
	line := pow.CreatePoWMessage(2, "#test", "hello")
	// A sender's made-up id and a far-future Timestamp are both ignored.
	hash, msg, ok := trustworthy(olnjson.Message{Raw: line, Timestamp: now.Add(100 * 24 * time.Hour)}, now)
	if !ok || hash != generateHash(line) || msg.Timestamp.After(now.Add(time.Minute)) {
		t.Fatalf("got %q %v %v", hash, msg.Timestamp, ok)
	}
	// A line mined for tomorrow (pre-mined for a mass release) is refused.
	b64 := "aGVsbG8"
	ahead := pow.POWEncode(2, pow.Version+";%d;"+now.Add(24*time.Hour).Format("20060102150405")+";"+b64+";#test")
	if _, _, ok := trustworthy(olnjson.Message{Raw: ahead}, now); ok {
		t.Fatal("a line dated tomorrow was accepted")
	}
	// An old line (relayed for days) is fine, and keeps its own date.
	old := pow.POWEncode(2, pow.Version+";%d;"+now.Add(-48*time.Hour).Format("20060102150405")+";"+b64+";#test")
	if _, msg, ok := trustworthy(olnjson.Message{Raw: old, Timestamp: now}, now); !ok || now.Sub(msg.Timestamp) < 47*time.Hour {
		t.Fatalf("old line: %v %v", msg.Timestamp, ok)
	}
}
