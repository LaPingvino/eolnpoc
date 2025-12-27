package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/lapingvino/eolnpoc/olnjson"
)

func main() {
	// Create an example OLN format
	format := olnjson.Format{
		Server: olnjson.ServerInfo{
			Link:       "https://example.com/oln",
			Name:       "Example Server",
			PubKey:     "example_public_key",
			AcceptPush: true,
		},
		Messages: make(map[string]olnjson.Message),
		Index:    make(map[string][]string),
		Feeds:    []string{},
		Push:     []string{},
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(format, "", "  ")
	if err != nil {
		log.Fatalf("Error marshaling JSON: %v", err)
	}

	fmt.Println(string(data))
}
