package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/lapingvino/eolnpoc/olnjson"
)

const (
	defaultNATSURL = "nats://demo.nats.io:4222"
	natsSubject    = "oln.messages.v1"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s [listen|publish|server] [args...]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nCommands:\n")
		fmt.Fprintf(os.Stderr, "  listen                    - Listen for OLN messages\n")
		fmt.Fprintf(os.Stderr, "  publish <message>         - Publish a message to OLN network\n")
		fmt.Fprintf(os.Stderr, "  server <nats-url>         - Set NATS server URL (default: %s)\n", defaultNATSURL)
		os.Exit(1)
	}

	command := os.Args[1]
	natsURL := defaultNATSURL

	switch command {
	case "listen":
		listenCommand(natsURL)
	case "publish":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: publish requires a message\n")
			os.Exit(1)
		}
		message := strings.Join(os.Args[2:], " ")
		publishCommand(natsURL, message)
	case "server":
		if len(os.Args) < 3 {
			fmt.Fprintf(os.Stderr, "Error: server requires a URL\n")
			os.Exit(1)
		}
		natsURL = os.Args[2]
		if len(os.Args) > 3 {
			command = os.Args[3]
			switch command {
			case "listen":
				listenCommand(natsURL)
			case "publish":
				if len(os.Args) < 5 {
					fmt.Fprintf(os.Stderr, "Error: publish requires a message\n")
					os.Exit(1)
				}
				message := strings.Join(os.Args[4:], " ")
				publishCommand(natsURL, message)
			default:
				fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
				os.Exit(1)
			}
		} else {
			fmt.Printf("NATS server set to: %s\n", natsURL)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		os.Exit(1)
	}
}

func connectNATS(url string) *nats.Conn {
	nc, err := nats.Connect(url)
	if err != nil {
		log.Fatalf("Failed to connect to NATS at %s: %v", url, err)
	}
	return nc
}

func extractHashtags(text string) []string {
	re := regexp.MustCompile(`#\w+`)
	matches := re.FindAllString(text, -1)
	uniqueTags := make(map[string]bool)
	for _, tag := range matches {
		uniqueTags[tag] = true
	}
	var tags []string
	for tag := range uniqueTags {
		tags = append(tags, tag)
	}
	return tags
}

func generateHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", hash)[:16] // Use first 16 chars for readability
}

func createMessage(text string) olnjson.Message {
	tags := extractHashtags(text)

	return olnjson.Message{
		Raw:       text,
		Timestamp: time.Now(),
		TTL:       7, // 7 days
		Hops:      0,
		Tags:      tags,
		Sig:       "", // TODO: signing
		Origin: olnjson.Origin{
			Display:    "anonymous",
			PubKey:     "",
			ServerName: "",
		},
	}
}

func publishCommand(natsURL, messageText string) {
	nc := connectNATS(natsURL)
	defer nc.Close()

	msg := createMessage(messageText)
	msgHash := generateHash(messageText)

	// Create OLN Format with the message
	format := olnjson.Format{
		Server: olnjson.ServerInfo{
			Link:       "oln.local",
			Name:       "OLN Node",
			PubKey:     "",
			AcceptPush: true,
		},
		Messages: map[string]olnjson.Message{
			msgHash: msg,
		},
		Index: make(map[string][]string),
		Feeds: []string{},
		Push:  []string{},
	}

	// Add tags to index
	for _, tag := range msg.Tags {
		format.Index[tag] = append(format.Index[tag], msgHash)
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(format)
	if err != nil {
		log.Fatalf("Failed to marshal message: %v", err)
	}

	// Publish to NATS
	err = nc.Publish(natsSubject, jsonData)
	if err != nil {
		log.Fatalf("Failed to publish message: %v", err)
	}

	fmt.Printf("Published: %s\n", messageText)
	fmt.Printf("Hash: %s\n", msgHash)
	if len(msg.Tags) > 0 {
		fmt.Printf("Tags: %s\n", strings.Join(msg.Tags, ", "))
	}
}

func displayMessage(format *olnjson.Format) {
	if len(format.Messages) == 0 {
		return
	}

	for hash, msg := range format.Messages {
		fmt.Printf("\n[%s] %s\n", msg.Timestamp.Format("2006-01-02 15:04:05"), hash)
		if len(msg.Tags) > 0 {
			fmt.Printf("  Tags: %s\n", strings.Join(msg.Tags, ", "))
		}
		if msg.Origin.Display != "" {
			fmt.Printf("  From: %s\n", msg.Origin.Display)
		}
		fmt.Printf("  %s\n", msg.Raw)
	}
}

func listenCommand(natsURL string) {
	nc := connectNATS(natsURL)
	defer nc.Close()

	fmt.Printf("Listening on %s for OLN messages...\n", natsSubject)
	fmt.Printf("Connected to: %s\n", natsURL)
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println(strings.Repeat("-", 60))

	_, err := nc.Subscribe(natsSubject, func(m *nats.Msg) {
		var format olnjson.Format
		err := json.Unmarshal(m.Data, &format)
		if err != nil {
			log.Printf("Error parsing message: %v", err)
			return
		}
		displayMessage(&format)
	})

	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}

	// Keep running
	select {}
}
