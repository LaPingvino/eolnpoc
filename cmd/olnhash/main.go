package main

import (
	"crypto/sha1"
	"encoding/base64"
	"time"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func POWEncode(bits int, format string) string {
	var sha [sha1.Size]byte
	var shab, check string
	for i := 0; i < bits; i++ {
		check += "0";
	}
	for i := 0; ;i++ {
		encode := fmt.Sprintf(format, i)
		sha = sha1.Sum([]byte(encode))
		shab = ""
		for _, el := range sha {
			shab += fmt.Sprintf("%08b", el)
		}
		if shab[:bits] == check {
			return fmt.Sprintf(format, i)
		}
	}
}

func main() {
	if len(os.Args) < 4 {
		panic("not enough arguments: requires bits keyword message")
	}
	var teststring = []byte(strings.Join(os.Args[3:], " "))
	var keyword string
	var date = time.Now().Format("20060102150405")
	var powbits, err = strconv.Atoi(os.Args[1])
	if err != nil {
		panic("error converting the proof of work strength in bits")
	}
	keyword = os.Args[2]
	var testencoded = base64.URLEncoding.EncodeToString(teststring)
	result := POWEncode(powbits, "%d;"+date+";"+testencoded+";"+keyword)
	fmt.Println(result, fmt.Sprintf("%x", sha1.Sum([]byte(result))))
}
