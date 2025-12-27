package olnjson

import "time"

// Format implements the OLN JSON message format specification.
// It represents the complete structure for OLN message passing between
// servers and peers.
type Format struct {
	Server   ServerInfo          `json:"server"`
	Messages map[string]Message  `json:"messages"`
	Index    map[string][]string `json:"index"`
	Feeds    []string            `json:"feeds"`
	Push     []string            `json:"push"`
}

// ServerInfo contains information about an OLN server.
type ServerInfo struct {
	Link       string `json:"link"`
	Name       string `json:"name"`
	PubKey     string `json:"pubkey"`
	AcceptPush bool   `json:"acceptpush"`
}

// Message represents a single OLN message.
type Message struct {
	Raw       string    `json:"raw"`
	Origin    Origin    `json:"origin"`
	Sig       string    `json:"sig"`
	Timestamp time.Time `json:"timestamp"`
	TTL       int       `json:"ttl"` // TTL in days
	Hops      int       `json:"hops"`
	Tags      []string  `json:"tags"`
}

// Origin identifies the source of a message.
type Origin struct {
	Display    string `json:"display"`
	PubKey     string `json:"pubkey"`
	ServerName string `json:"servername"`
}
