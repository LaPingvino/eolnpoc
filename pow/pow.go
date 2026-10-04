// Package pow is OLN's proof of work, format v2 (memory-hard).
//
// A message line is
//
//	v2;<nonce>;<YYYYMMDDhhmmss UTC>;<base64url(message)>;<keywords>
//
// and its work is the number of leading zero bits of
// Argon2id(line, salt "OLN-v2-proofwork", 1 pass, 4 MiB, 1 lane, 32 bytes).
// Argon2id needs memory per try, so a graphics card gains little over a
// phone (with SHA-1, v1, the gap was about a millionfold). The message id
// is the hex SHA-1 of the line. What a node accepts, keeps or drops is the
// node's own policy: OLN fixes the format, not the policy.
package pow

import (
	"encoding/base64"
	"fmt"
	"math/bits"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

// Version is the line prefix this package reads and writes.
const Version = "v2"

var salt = []byte("OLN-v2-proofwork")

// Work returns the leading zero bits of the line's Argon2id hash (0 for a
// line that isn't v2).
func Work(line string) int {
	if !strings.HasPrefix(line, Version+";") {
		return 0
	}
	h := argon2.IDKey([]byte(line), salt, 1, 4096, 1, 32)
	n := 0
	for _, b := range h {
		if b == 0 {
			n += 8
			continue
		}
		return n + bits.LeadingZeros8(b)
	}
	return n
}

// POWEncode finds a nonce for format (with one %d for the nonce, and the
// "v2;" prefix included) giving at least want bits of work.
func POWEncode(want int, format string) string {
	for i := 0; ; i++ {
		if line := fmt.Sprintf(format, i); Work(line) >= want {
			return line
		}
	}
}

// CreatePoWMessage makes a v2 line for message and keyword(s) with at least
// want bits of work; the date is UTC (it's part of the work).
func CreatePoWMessage(want int, keyword, message string) string {
	b64 := base64.URLEncoding.EncodeToString([]byte(message))
	date := time.Now().UTC().Format("20060102150405")
	return POWEncode(want, Version+";%d;"+date+";"+b64+";"+keyword)
}

// ParsePoWMessage splits a v2 line: (nonce, date, message, keywords, error).
func ParsePoWMessage(encoded string) (string, string, string, string, error) {
	if !strings.HasPrefix(encoded, Version+";") {
		return "", "", "", "", fmt.Errorf("not an OLN %s message", Version)
	}
	parts := strings.Split(strings.TrimPrefix(encoded, Version+";"), ";")
	if len(parts) < 4 {
		return "", "", "", "", fmt.Errorf("invalid PoW message format")
	}
	msg, err := base64.URLEncoding.DecodeString(parts[2])
	if err != nil {
		if msg, err = base64.RawURLEncoding.DecodeString(parts[2]); err != nil {
			return "", "", "", "", fmt.Errorf("failed to decode message: %v", err)
		}
	}
	return parts[0], parts[1], string(msg), strings.Join(parts[3:], ";"), nil
}

// ValidatePoW returns the work (leading zero bits) of a line; see Work.
func ValidatePoW(encoded string) int { return Work(encoded) }
