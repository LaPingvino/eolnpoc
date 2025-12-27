package pow

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

// POWEncode performs proof-of-work encoding by finding a nonce that,
// when combined with the format string, produces a hash with the
// specified number of leading zero bits.
func POWEncode(bits int, format string) string {
	check := strings.Repeat("0", bits)

	for i := 0; ; i++ {
		encode := fmt.Sprintf(format, i)
		sha := sha1.Sum([]byte(encode))

		// Convert hash to binary string
		var shab strings.Builder
		for _, el := range sha {
			shab.WriteString(fmt.Sprintf("%08b", el))
		}

		// Check if we have enough leading zeros
		if strings.HasPrefix(shab.String(), check) {
			return encode
		}
	}
}

// CreatePoWMessage generates a PoW-encoded message in the format:
// <nonce>;<date>;<base64_message>;<keyword>
func CreatePoWMessage(bits int, keyword, message string) string {
	messageEncoded := base64.URLEncoding.EncodeToString([]byte(message))
	date := time.Now().Format("20060102150405")
	format := "%d;" + date + ";" + messageEncoded + ";" + keyword
	return POWEncode(bits, format)
}

// ParsePoWMessage parses a PoW-encoded message and returns:
// (nonce, date, message, keyword, error)
func ParsePoWMessage(encoded string) (string, string, string, string, error) {
	parts := strings.Split(encoded, ";")
	if len(parts) < 4 {
		return "", "", "", "", fmt.Errorf("invalid PoW message format")
	}

	nonce := parts[0]
	date := parts[1]
	messageB64 := parts[2]
	keyword := strings.Join(parts[3:], ";") // Handle keywords with semicolons

	messageBytes, err := base64.URLEncoding.DecodeString(messageB64)
	if err != nil {
		return "", "", "", "", fmt.Errorf("failed to decode message: %v", err)
	}

	return nonce, date, string(messageBytes), keyword, nil
}

// ValidatePoW checks if a PoW message has the required number of leading zero bits.
// Returns the number of leading zero bits found.
func ValidatePoW(encoded string) int {
	hash := sha1.Sum([]byte(encoded))

	// Convert hash to binary string
	var binary strings.Builder
	for _, el := range hash {
		binary.WriteString(fmt.Sprintf("%08b", el))
	}

	// Count leading zeros
	binStr := binary.String()
	leadingZeros := 0
	for _, bit := range binStr {
		if bit == '0' {
			leadingZeros++
		} else {
			break
		}
	}

	return leadingZeros
}
