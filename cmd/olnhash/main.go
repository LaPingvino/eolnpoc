// Command olnhash makes an OLN v2 message line on the command line:
//
//	olnhash <bits> <keyword(s)> <message...>
//
// prints the line and its id (hex SHA-1 of the line).
package main

import (
	"crypto/sha1"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/lapingvino/eolnpoc/pow"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintf(os.Stderr, "Usage: %s <bits> <keyword> <message...>\n", os.Args[0])
		os.Exit(1)
	}
	bits, err := strconv.Atoi(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid bits value: %v\n", err)
		os.Exit(1)
	}
	line := pow.CreatePoWMessage(bits, os.Args[2], strings.Join(os.Args[3:], " "))
	fmt.Printf("%s %x\n", line, sha1.Sum([]byte(line)))
}
