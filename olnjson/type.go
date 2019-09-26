package olnjson

import "time"

// The Format type implements a first model of
// the suggested JSON structure for OLN
type Format struct {
	Server struct {
		Link       string
		Name       string
		PubKey     string
		AcceptPush bool
	}
	Messages map[string]struct {
		Raw    string
		Origin struct {
			Display    string
			PubKey     string
			ServerName string
		}
		Sig       string
		Timestamp time.Time
		TTL       time.Duration
		Hops      int
		Tags      []string
	}
	Index map[string][]string
	Feeds []string
	Push  []string
}
