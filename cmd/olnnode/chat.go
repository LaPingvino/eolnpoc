package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/lapingvino/eolnpoc/location"
	"github.com/lapingvino/eolnpoc/olnjson"
	"github.com/lapingvino/eolnpoc/pow"
)

const (
	maxHops            = 3
	defaultMaxCache    = 100
	defaultRebroadcast = 5 * time.Minute
	ttlDays            = 7
)

// MessageEntry wraps a message with metadata for prioritization
type MessageEntry struct {
	Hash           string
	Message        olnjson.Message
	Priority       int
	PoWBits        int
	Plustags       []string // Extracted location codes
	ProximityScore int      // Based on user's location
	FirstSeen      time.Time
	LastSent       time.Time
}

// ChatFilters defines user preferences
type ChatFilters struct {
	Hashtags  []string
	Locations []string
}

// ChatState manages the chat session state
type ChatState struct {
	Cache               map[string]*MessageEntry
	Filters             ChatFilters
	NC                  *nats.Conn
	MaxCacheSize        int
	RebroadcastInterval time.Duration
	AutoPoWBits         int
	mu                  sync.RWMutex
	stopChan            chan bool
}

func chatCommand(natsURL string, args []string) {
	fs := flag.NewFlagSet("chat", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: olnnode chat [options]\n")
		fs.PrintDefaults()
	}

	var tags, locations, server string
	var maxCache int
	var rebroadcast string
	var autoPow int

	fs.StringVar(&tags, "tag", "", "Comma-separated hashtags to filter (e.g., #OLN,#test)")
	fs.StringVar(&locations, "location", "", "Location filter (pluscode format)")
	fs.IntVar(&maxCache, "max-cache", defaultMaxCache, "Max messages to cache")
	fs.StringVar(&rebroadcast, "rebroadcast", "5m", "Rebroadcast interval")
	fs.IntVar(&autoPow, "auto-pow", 0, "Auto-apply N-bit PoW to all messages")
	fs.StringVar(&server, "server", "", "NATS server URL")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if server == "" {
		server = natsURL
	}

	// Parse rebroadcast interval
	rebroadcastDur, err := time.ParseDuration(rebroadcast)
	if err != nil {
		log.Fatalf("Invalid rebroadcast interval: %v", err)
	}

	// Parse filters
	var hashtags []string
	if tags != "" {
		for _, tag := range strings.Split(tags, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				hashtags = append(hashtags, tag)
			}
		}
	}

	var locFilters []string
	if locations != "" {
		for _, loc := range strings.Split(locations, ",") {
			loc = strings.TrimSpace(loc)
			if loc != "" {
				locFilters = append(locFilters, loc)
			}
		}
	}

	// Create chat state
	state := &ChatState{
		Cache:               make(map[string]*MessageEntry),
		Filters:             ChatFilters{Hashtags: hashtags, Locations: locFilters},
		MaxCacheSize:        maxCache,
		RebroadcastInterval: rebroadcastDur,
		AutoPoWBits:         autoPow,
		stopChan:            make(chan bool),
	}

	// Connect to NATS
	nc := connectNATS(server)
	defer nc.Close()
	state.NC = nc

	fmt.Printf("OLN Chat Mode (%s)\n", server)
	if len(hashtags) > 0 {
		fmt.Printf("Hashtag filters: %s\n", strings.Join(hashtags, ", "))
	}
	if len(locFilters) > 0 {
		fmt.Printf("Location filters: %s\n", strings.Join(locFilters, ", "))
	}
	fmt.Println("Type messages and press Enter to send. Type !help for commands. Ctrl+C to exit.")
	fmt.Println(strings.Repeat("-", 60))

	// Start message receiver
	go state.messageReceiver()

	// Start rebroadcast timer
	go state.rebroadcastLoop()

	// Start cleanup timer
	go state.cleanupLoop()

	// Start input handler
	state.handleInput()

	// Cleanup
	close(state.stopChan)
}

func (s *ChatState) messageReceiver() {
	sub, err := s.NC.Subscribe(natsSubject, func(m *nats.Msg) {
		var format olnjson.Format
		if err := json.Unmarshal(m.Data, &format); err != nil {
			return
		}

		for hash, msg := range format.Messages {
			s.addMessage(hash, msg)
		}
	})
	if err != nil {
		log.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	<-s.stopChan
}

func (s *ChatState) addMessage(hash string, msg olnjson.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Skip if already cached
	if _, exists := s.Cache[hash]; exists {
		return
	}

	// Detect PoW
	powBits := s.detectPoW(msg.Raw)

	// Extract plustags (both direct and from #geo hashtags)
	plustags := location.AllPlustags(msg.Raw)

	// Calculate proximity score
	proximityScore := 0
	if len(s.Filters.Locations) > 0 && len(plustags) > 0 {
		// Use the best proximity score among all message locations
		for _, msgLoc := range plustags {
			for _, userLoc := range s.Filters.Locations {
				score := location.CalculateProximity(msgLoc, userLoc)
				if score > proximityScore {
					proximityScore = score
				}
			}
		}
	}

	// Calculate priority
	priority := s.calculatePriority(msg, powBits, proximityScore)

	entry := &MessageEntry{
		Hash:           hash,
		Message:        msg,
		Priority:       priority,
		PoWBits:        powBits,
		Plustags:       plustags,
		ProximityScore: proximityScore,
		FirstSeen:      time.Now(),
		LastSent:       time.Now(),
	}

	s.Cache[hash] = entry

	// Display message
	s.displayMessage(hash, entry)

	// Evict lowest priority if cache is full
	if len(s.Cache) > s.MaxCacheSize {
		s.evictLowestPriority()
	}
}

func (s *ChatState) displayMessage(hash string, entry *MessageEntry) {
	msg := entry.Message
	indicator := ""

	if s.matchesFilters(msg) {
		indicator = " [★]"
	}

	// Location indicator
	if entry.ProximityScore > 0 {
		if entry.ProximityScore >= 500 {
			indicator += " [📍 exact]"
		} else if entry.ProximityScore >= 250 {
			indicator += " [📍 nearby]"
		} else {
			indicator += " [📍 region]"
		}
	}

	if entry.PoWBits > 0 {
		indicator += fmt.Sprintf(" [PoW:%d]", entry.PoWBits)
	}

	fmt.Printf("\n[%s] %s%s\n", msg.Timestamp.Format("2006-01-02 15:04:05"), hash[:8], indicator)

	// Show all tags including plustags
	allTags := msg.Tags
	for _, plustag := range entry.Plustags {
		// Check if plustag is already in tags
		found := false
		for _, t := range allTags {
			if t == plustag {
				found = true
				break
			}
		}
		if !found {
			allTags = append(allTags, plustag)
		}
	}

	if len(allTags) > 0 {
		fmt.Printf("  Tags: %s\n", strings.Join(allTags, ", "))
	}
	if msg.Origin.Display != "" {
		fmt.Printf("  From: %s\n", msg.Origin.Display)
	}
	fmt.Printf("  %s\n", msg.Raw)
	fmt.Print("> ")
}

func (s *ChatState) calculatePriority(msg olnjson.Message, powBits int, proximityScore int) int {
	priority := 100 // BaseScore

	// FilterBonus
	if s.matchesFilters(msg) {
		priority += 1000
	}

	// ProximityScore (if user has a location filter)
	priority += proximityScore

	// RecencyScore (TTL remaining as percentage)
	age := time.Since(msg.Timestamp)
	ttlDuration := time.Duration(msg.TTL) * 24 * time.Hour
	if age < ttlDuration {
		remaining := 1.0 - (float64(age) / float64(ttlDuration))
		priority += int(remaining * 100)
	}

	// PoWScore
	priority += powBits * 50

	// HopsScore (negative)
	priority -= msg.Hops * 10

	return priority
}

func (s *ChatState) matchesFilters(msg olnjson.Message) bool {
	if len(s.Filters.Hashtags) == 0 && len(s.Filters.Locations) == 0 {
		return false
	}

	// Check hashtags
	for _, filterTag := range s.Filters.Hashtags {
		for _, msgTag := range msg.Tags {
			if strings.EqualFold(filterTag, msgTag) {
				return true
			}
		}
	}

	// Check locations (simple substring match for pluscodes)
	if len(s.Filters.Locations) > 0 {
		msgText := msg.Raw
		for _, locFilter := range s.Filters.Locations {
			if strings.Contains(msgText, locFilter) {
				return true
			}
		}
	}

	return false
}

func (s *ChatState) detectPoW(msgText string) int {
	// Check if message looks like PoW format
	parts := strings.Split(msgText, ";")
	if len(parts) < 4 {
		return 0
	}

	// Try to parse as PoW message
	_, _, _, _, err := pow.ParsePoWMessage(msgText)
	if err != nil {
		return 0
	}

	// Validate PoW
	powBits := pow.ValidatePoW(msgText)
	return powBits
}

func (s *ChatState) evictLowestPriority() {
	var lowest *MessageEntry
	var lowestHash string

	for hash, entry := range s.Cache {
		if lowest == nil || entry.Priority < lowest.Priority {
			lowest = entry
			lowestHash = hash
		}
	}

	if lowestHash != "" {
		delete(s.Cache, lowestHash)
	}
}

func (s *ChatState) rebroadcastLoop() {
	ticker := time.NewTicker(s.RebroadcastInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.rebroadcastMessages()
		}
	}
}

func (s *ChatState) rebroadcastMessages() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()

	for hash, entry := range s.Cache {
		msg := entry.Message

		// Check if message is still valid
		if msg.Hops >= maxHops {
			continue
		}

		age := now.Sub(msg.Timestamp)
		ttlDuration := time.Duration(msg.TTL) * 24 * time.Hour

		// Don't rebroadcast if expired
		if age > ttlDuration {
			continue
		}

		// Only rebroadcast if >50% TTL remaining
		if age > ttlDuration/2 {
			continue
		}

		// Increment hops and rebroadcast
		msg.Hops++
		entry.LastSent = now

		format := olnjson.Format{
			Server: olnjson.ServerInfo{
				Link:       "oln.local",
				Name:       "OLN Node",
				PubKey:     "",
				AcceptPush: true,
			},
			Messages: map[string]olnjson.Message{
				hash: msg,
			},
			Index: make(map[string][]string),
			Feeds: []string{},
			Push:  []string{},
		}

		jsonData, err := json.Marshal(format)
		if err != nil {
			continue
		}

		s.NC.Publish(natsSubject, jsonData)
	}
}

func (s *ChatState) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.cleanupExpired()
		}
	}
}

func (s *ChatState) cleanupExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	for hash, entry := range s.Cache {
		age := now.Sub(entry.Message.Timestamp)
		ttlDuration := time.Duration(entry.Message.TTL) * 24 * time.Hour

		if age > ttlDuration {
			delete(s.Cache, hash)
		}
	}
}

func (s *ChatState) handleInput() {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Print("> ")

	for scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())

		if input == "" {
			fmt.Print("> ")
			continue
		}

		if strings.HasPrefix(input, "!") {
			s.handleCommand(input)
		} else {
			s.publishMessage(input, 0)
		}

		fmt.Print("> ")
	}
}

func (s *ChatState) handleCommand(input string) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return
	}

	cmd := parts[0]

	switch cmd {
	case "!pow":
		if len(parts) < 3 {
			fmt.Println("Usage: !pow <bits> <message>")
			return
		}
		bits, err := strconv.Atoi(parts[1])
		if err != nil {
			fmt.Println("Invalid bits value")
			return
		}
		message := strings.Join(parts[2:], " ")
		s.publishMessage(message, bits)

	case "!list":
		s.listMessages(parts[1:])

	case "!filter":
		s.handleFilterCommand(parts[1:])

	case "!stats":
		s.showStats()

	case "!show":
		if len(parts) < 2 {
			fmt.Println("Usage: !show <hash>")
			return
		}
		s.showMessage(parts[1])

	case "!clear":
		s.clearCache()

	case "!search":
		s.searchMessages(parts[1:])

	case "!help":
		fmt.Println("Commands:")
		fmt.Println("  !pow <bits> <message>       - Send message with proof-of-work")
		fmt.Println("  !list [N|full]              - List cached messages (top N or full text)")
		fmt.Println("  !filter add tag <tags>      - Add hashtag filter(s)")
		fmt.Println("  !filter add location <code> - Add location filter")
		fmt.Println("  !filter remove tag <tag>    - Remove hashtag filter")
		fmt.Println("  !filter remove location     - Remove location filters")
		fmt.Println("  !filter clear               - Clear all filters")
		fmt.Println("  !filter show                - Show active filters")
		fmt.Println("  !search <query>             - Search messages by text/tags/location")
		fmt.Println("  !search tag <hashtag>       - Search by specific hashtag")
		fmt.Println("  !search location <code>     - Search by location proximity")
		fmt.Println("  !search text <keywords>     - Search only in message text")
		fmt.Println("  !stats                      - Show cache statistics")
		fmt.Println("  !show <hash>                - Show full message details")
		fmt.Println("  !clear                      - Clear message cache")
		fmt.Println("  !help                       - Show this help")

	default:
		fmt.Println("Unknown command. Type !help for commands.")
	}
}

func (s *ChatState) handleFilterCommand(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: !filter <add|remove|clear|show> ...")
		return
	}

	action := args[0]

	switch action {
	case "add":
		if len(args) < 3 {
			fmt.Println("Usage: !filter add <tag|location> <value>")
			return
		}
		filterType := args[1]
		value := strings.Join(args[2:], " ")

		if filterType == "tag" {
			s.addHashtagFilter(value)
		} else if filterType == "location" {
			s.addLocationFilter(value)
		} else {
			fmt.Println("Unknown filter type. Use 'tag' or 'location'")
		}

	case "remove":
		if len(args) < 2 {
			fmt.Println("Usage: !filter remove <tag|location> [value]")
			return
		}
		filterType := args[1]

		if filterType == "tag" && len(args) >= 3 {
			s.removeHashtagFilter(args[2])
		} else if filterType == "location" {
			s.removeLocationFilter()
		} else {
			fmt.Println("Usage: !filter remove <tag|location> [value]")
		}

	case "clear":
		s.clearFilters()

	case "show":
		s.showFilters()

	default:
		fmt.Println("Unknown filter action. Use: add, remove, clear, or show")
	}
}

func (s *ChatState) addHashtagFilter(tags string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, tag := range strings.Split(tags, ",") {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			// Check if already exists
			found := false
			for _, existing := range s.Filters.Hashtags {
				if existing == tag {
					found = true
					break
				}
			}
			if !found {
				s.Filters.Hashtags = append(s.Filters.Hashtags, tag)
				fmt.Printf("Added tag filter: %s\n", tag)
			}
		}
	}
	s.recalculatePriorities()
}

func (s *ChatState) removeHashtagFilter(tag string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, t := range s.Filters.Hashtags {
		if t == tag {
			s.Filters.Hashtags = append(s.Filters.Hashtags[:i], s.Filters.Hashtags[i+1:]...)
			fmt.Printf("Removed tag filter: %s\n", tag)
			s.recalculatePriorities()
			return
		}
	}
	fmt.Printf("Filter not found: %s\n", tag)
}

func (s *ChatState) addLocationFilter(locationCode string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	locationCode = strings.TrimSpace(locationCode)
	// Accept any location code format (will validate in proximity calculation)
	if !location.ValidatePluscode(locationCode) {
		fmt.Printf("Warning: '%s' may not be a valid pluscode\n", locationCode)
	}

	// Check if already exists
	for _, existing := range s.Filters.Locations {
		if existing == locationCode {
			fmt.Printf("Location filter already exists: %s\n", locationCode)
			return
		}
	}

	s.Filters.Locations = append(s.Filters.Locations, locationCode)
	fmt.Printf("Added location filter: %s\n", locationCode)
	s.recalculatePriorities()
}

func (s *ChatState) removeLocationFilter() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.Filters.Locations) == 0 {
		fmt.Println("No location filters to remove")
		return
	}

	fmt.Printf("Removed %d location filter(s)\n", len(s.Filters.Locations))
	s.Filters.Locations = []string{}
	s.recalculatePriorities()
}

func (s *ChatState) clearFilters() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Filters.Hashtags = []string{}
	s.Filters.Locations = []string{}
	fmt.Println("All filters cleared")
	s.recalculatePriorities()
}

func (s *ChatState) showFilters() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.Filters.Hashtags) == 0 && len(s.Filters.Locations) == 0 {
		fmt.Println("No active filters")
		return
	}

	if len(s.Filters.Hashtags) > 0 {
		fmt.Printf("Hashtag filters: %s\n", strings.Join(s.Filters.Hashtags, ", "))
	}
	if len(s.Filters.Locations) > 0 {
		fmt.Printf("Location filters: %s\n", strings.Join(s.Filters.Locations, ", "))
	}
}

func (s *ChatState) recalculatePriorities() {
	for _, entry := range s.Cache {
		// Recalculate proximity if location filters changed
		proximityScore := 0
		if len(s.Filters.Locations) > 0 && len(entry.Plustags) > 0 {
			for _, msgLoc := range entry.Plustags {
				for _, userLoc := range s.Filters.Locations {
					score := location.CalculateProximity(msgLoc, userLoc)
					if score > proximityScore {
						proximityScore = score
					}
				}
			}
		}
		entry.ProximityScore = proximityScore

		// Recalculate priority
		entry.Priority = s.calculatePriority(entry.Message, entry.PoWBits, proximityScore)
	}
}

func (s *ChatState) showStats() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	fmt.Printf("Cache: %d/%d messages\n", len(s.Cache), s.MaxCacheSize)

	if len(s.Filters.Hashtags) > 0 || len(s.Filters.Locations) > 0 {
		fmt.Print("Filters: ")
		if len(s.Filters.Hashtags) > 0 {
			fmt.Print(strings.Join(s.Filters.Hashtags, ", "))
		}
		if len(s.Filters.Locations) > 0 {
			if len(s.Filters.Hashtags) > 0 {
				fmt.Print(" | ")
			}
			fmt.Print(strings.Join(s.Filters.Locations, ", "))
		}
		fmt.Println()
	} else {
		fmt.Println("Filters: none")
	}

	if len(s.Cache) > 0 {
		var totalAge time.Duration
		var minPriority, maxPriority int
		first := true

		for _, entry := range s.Cache {
			totalAge += time.Since(entry.FirstSeen)
			if first {
				minPriority = entry.Priority
				maxPriority = entry.Priority
				first = false
			} else {
				if entry.Priority < minPriority {
					minPriority = entry.Priority
				}
				if entry.Priority > maxPriority {
					maxPriority = entry.Priority
				}
			}
		}

		avgAge := totalAge / time.Duration(len(s.Cache))
		fmt.Printf("Average age: %s\n", avgAge.Round(time.Second))
		fmt.Printf("Priority range: %d-%d\n", minPriority, maxPriority)
	}
}

func (s *ChatState) showMessage(hashPrefix string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Find message by hash prefix
	for hash, entry := range s.Cache {
		if strings.HasPrefix(hash, hashPrefix) {
			msg := entry.Message
			indicator := s.buildIndicators(entry)

			fmt.Printf("\n[%s] %s%s\n", msg.Timestamp.Format("2006-01-02 15:04:05"), hash, indicator)
			fmt.Printf("Priority: %d\n", entry.Priority)
			fmt.Printf("Age: %s\n", time.Since(msg.Timestamp).Round(time.Second))

			if len(msg.Tags) > 0 {
				fmt.Printf("Tags: %s\n", strings.Join(msg.Tags, ", "))
			}
			if msg.Origin.Display != "" {
				fmt.Printf("From: %s\n", msg.Origin.Display)
			}
			fmt.Printf("\n%s\n", msg.Raw)
			return
		}
	}

	fmt.Printf("Message not found: %s\n", hashPrefix)
}

func (s *ChatState) clearCache() {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := len(s.Cache)
	s.Cache = make(map[string]*MessageEntry)
	fmt.Printf("Cleared %d messages from cache\n", count)
}

func (s *ChatState) searchMessages(args []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(args) == 0 {
		fmt.Println("Usage: !search <query> | !search tag <tag> | !search location <code> | !search text <keywords>")
		return
	}

	var matches []struct {
		hash  string
		entry *MessageEntry
	}

	mode := "default"
	query := strings.Join(args, " ")

	// Detect search mode
	if len(args) >= 2 {
		if args[0] == "tag" {
			mode = "tag"
			query = strings.Join(args[1:], " ")
		} else if args[0] == "location" {
			mode = "location"
			query = strings.Join(args[1:], " ")
		} else if args[0] == "text" {
			mode = "text"
			query = strings.Join(args[1:], " ")
		}
	}

	queryLower := strings.ToLower(query)

	// Search through cache
	for hash, entry := range s.Cache {
		match := false

		switch mode {
		case "tag":
			// Search for exact tag match
			for _, tag := range entry.Message.Tags {
				if strings.EqualFold(tag, query) {
					match = true
					break
				}
			}

		case "location":
			// Use proximity scoring for location matching
			for _, plustag := range entry.Plustags {
				if location.CalculateProximity(plustag, query) > 0 {
					match = true
					break
				}
			}

		case "text":
			// Search only in message text
			if strings.Contains(strings.ToLower(entry.Message.Raw), queryLower) {
				match = true
			}

		default:
			// Search in message text
			if strings.Contains(strings.ToLower(entry.Message.Raw), queryLower) {
				match = true
			}
			// Search in tags
			if !match {
				for _, tag := range entry.Message.Tags {
					if strings.Contains(strings.ToLower(tag), queryLower) {
						match = true
						break
					}
				}
			}
			// Search in plustags
			if !match {
				for _, plustag := range entry.Plustags {
					if strings.Contains(strings.ToLower(plustag), queryLower) {
						match = true
						break
					}
				}
			}
		}

		if match {
			matches = append(matches, struct {
				hash  string
				entry *MessageEntry
			}{hash, entry})
		}
	}

	if len(matches) == 0 {
		fmt.Printf("No messages found for: %s\n", query)
		return
	}

	// Sort by priority (most relevant first)
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].entry.Priority > matches[j].entry.Priority
	})

	fmt.Printf("Found %d message(s) for: %s\n", len(matches), query)
	for i, m := range matches {
		indicator := s.buildIndicators(m.entry)
		age := time.Since(m.entry.Message.Timestamp)

		fmt.Printf("%d. [%s] priority: %d, age: %s%s\n",
			i+1, m.hash[:8], m.entry.Priority, age.Round(time.Second), indicator)

		if len(m.entry.Message.Tags) > 0 {
			fmt.Printf("   Tags: %s\n", strings.Join(m.entry.Message.Tags, ", "))
		}

		text := m.entry.Message.Raw
		if len(text) > 70 {
			text = text[:70] + "..."
		}
		fmt.Printf("   \"%s\"\n", text)
	}
}

func (s *ChatState) publishMessage(messageText string, powBits int) {
	var finalMessage string
	var msgHash string

	if powBits > 0 {
		fmt.Printf("Computing proof-of-work (%d bits)...\n", powBits)
		finalMessage = pow.CreatePoWMessage(powBits, "oln", messageText)
		msgHash = generateHash(finalMessage)
	} else if s.AutoPoWBits > 0 {
		fmt.Printf("Applying auto PoW (%d bits)...\n", s.AutoPoWBits)
		finalMessage = pow.CreatePoWMessage(s.AutoPoWBits, "oln", messageText)
		msgHash = generateHash(finalMessage)
	} else {
		finalMessage = messageText
		msgHash = generateHash(finalMessage)
	}

	// Create message
	tags := extractHashtags(finalMessage)

	// Extract plustags and geo hashtags
	plustags := location.AllPlustags(finalMessage)
	allTags := make([]string, len(tags))
	copy(allTags, tags)
	allTags = append(allTags, plustags...)

	msg := olnjson.Message{
		Raw:       finalMessage,
		Timestamp: time.Now(),
		TTL:       ttlDays,
		Hops:      0,
		Tags:      allTags,
		Sig:       "",
		Origin: olnjson.Origin{
			Display:    "anonymous",
			PubKey:     "",
			ServerName: "",
		},
	}

	// Create format
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

	// Add regular tags to index
	for _, tag := range tags {
		format.Index[tag] = append(format.Index[tag], msgHash)
	}

	// Add plustags and their hierarchy to index
	for _, plustag := range plustags {
		parents := location.GetParentPlustags(plustag)
		for _, parent := range parents {
			format.Index[parent] = append(format.Index[parent], msgHash)
		}
	}

	// Marshal and publish
	jsonData, err := json.Marshal(format)
	if err != nil {
		fmt.Printf("Error marshaling message: %v\n", err)
		return
	}

	err = s.NC.Publish(natsSubject, jsonData)
	if err != nil {
		fmt.Printf("Error publishing message: %v\n", err)
		return
	}

	fmt.Printf("Published (hash: %s)\n", msgHash[:8])
}

func (s *ChatState) listMessages(args []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.Cache) == 0 {
		fmt.Println("No messages cached")
		return
	}

	// Parse options
	limit := 0
	fullText := false

	for _, arg := range args {
		if arg == "full" {
			fullText = true
		} else if num, err := strconv.Atoi(arg); err == nil && num > 0 {
			limit = num
		}
	}

	// Sort by priority
	type sortEntry struct {
		hash  string
		entry *MessageEntry
	}

	var entries []sortEntry
	for hash, entry := range s.Cache {
		entries = append(entries, sortEntry{hash, entry})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].entry.Priority > entries[j].entry.Priority
	})

	if limit > 0 && limit < len(entries) {
		entries = entries[:limit]
	}

	fmt.Printf("Cached messages (%d/%d):\n", len(entries), len(s.Cache))
	for i, e := range entries {
		indicator := s.buildIndicators(e.entry)

		age := time.Since(e.entry.Message.Timestamp)
		fmt.Printf("%d. [%s] priority: %d, age: %s%s\n",
			i+1, e.hash[:8], e.entry.Priority, age.Round(time.Second), indicator)

		// Show tags
		if len(e.entry.Message.Tags) > 0 {
			fmt.Printf("   Tags: %s\n", strings.Join(e.entry.Message.Tags, ", "))
		}

		// Show message text
		text := e.entry.Message.Raw
		if !fullText && len(text) > 70 {
			text = text[:70] + "..."
		}
		fmt.Printf("   \"%s\"\n", text)
	}
}

func (s *ChatState) buildIndicators(entry *MessageEntry) string {
	indicator := ""

	if s.matchesFilters(entry.Message) {
		indicator = " [★]"
	}

	if entry.ProximityScore > 0 {
		if entry.ProximityScore >= 500 {
			indicator += " [📍 exact]"
		} else if entry.ProximityScore >= 250 {
			indicator += " [📍 nearby]"
		} else {
			indicator += " [📍 region]"
		}
	}

	if entry.PoWBits > 0 {
		indicator += fmt.Sprintf(" [PoW:%d]", entry.PoWBits)
	}

	return indicator
}
