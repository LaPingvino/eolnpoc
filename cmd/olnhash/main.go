package main

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// POWEncode performs proof-of-work encoding by finding a nonce
// that when combined with the format string produces a hash with
// the specified number of leading zero bits.
func POWEncode(bits int, format string) string {
	// Build check string: "000...0" with 'bits' zeros
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

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintf(os.Stderr, "Usage: %s <bits> <keyword> <message...>\n", os.Args[0])
		os.Exit(1)
	}

	powbits, err := strconv.Atoi(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid bits value: %v\n", err)
		os.Exit(1)
	}

	keyword := os.Args[2]
	messageString := strings.Join(os.Args[3:], " ")
	messageEncoded := base64.URLEncoding.EncodeToString([]byte(messageString))
	date := time.Now().Format("20060102150405")

	format := "%d;" + date + ";" + messageEncoded + ";" + keyword
	result := POWEncode(powbits, format)
	hash := sha1.Sum([]byte(result))

	fmt.Printf("%s %x\n", result, hash)
}
