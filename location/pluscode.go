package location

import (
	"regexp"
	"strings"
)

// Base20 charset used in pluscodes (OLC/Plus Codes)
const base20 = "23456789CFGHJMPQRVWX"

// ValidatePluscode checks if a string is a valid pluscode format
func ValidatePluscode(code string) bool {
	// Remove spaces if any
	code = strings.TrimSpace(code)

	// Must contain a plus sign
	if !strings.Contains(code, "+") {
		return false
	}

	parts := strings.Split(code, "+")
	if len(parts) != 2 {
		return false
	}

	prefix := parts[0]
	suffix := parts[1]

	// Prefix should be 8 base20 chars
	if len(prefix) != 8 {
		return false
	}

	// Suffix should be 0-2 base20 chars
	if len(suffix) > 2 {
		return false
	}

	// All chars should be base20
	for _, c := range prefix {
		if !strings.ContainsRune(base20, c) {
			return false
		}
	}

	for _, c := range suffix {
		if !strings.ContainsRune(base20, c) {
			return false
		}
	}

	return true
}

// ExtractPluscodes finds all pluscodes in text
func ExtractPluscodes(text string) []string {
	// Regex for pluscode: 8 base20 chars + '+' + 0-2 base20 chars
	// We use a simplified pattern that matches the format
	re := regexp.MustCompile(`[23456789CFGHJMPQRVWX]{8}\+[23456789CFGHJMPQRVWX]{0,2}`)
	matches := re.FindAllString(text, -1)

	// Deduplicate
	seen := make(map[string]bool)
	var result []string
	for _, m := range matches {
		if !seen[m] && ValidatePluscode(m) {
			seen[m] = true
			result = append(result, m)
		}
	}

	return result
}

// ExtractGeoHashtags converts #geoXXXXXXXX hashtags to pluscodes
// e.g., #geo6FG22222 → 6FG22222+
func ExtractGeoHashtags(text string) []string {
	// Look for #geo followed by 8 base20 chars
	re := regexp.MustCompile(`#geo([23456789CFGHJMPQRVWX]{8})`)
	matches := re.FindAllStringSubmatch(text, -1)

	var result []string
	seen := make(map[string]bool)

	for _, match := range matches {
		if len(match) > 1 {
			code := match[1] + "+"
			if !seen[code] {
				seen[code] = true
				result = append(result, code)
			}
		}
	}

	return result
}

// PadPluscodePair pads a 2-char pair with 00
// e.g., "22" → "00"
func PadPluscodePair(pair string) string {
	if len(pair) < 2 {
		return "00"
	}
	return "00"
}

// GetParentPlustags generates the hierarchy of pluscodes
// e.g., 6FG22222+22 → [6FG22222+22, 6FG22222+, 6FG22200+, 6FG20000+, 6FG00000+]
func GetParentPlustags(code string) []string {
	if !ValidatePluscode(code) {
		return []string{}
	}

	parts := strings.Split(code, "+")
	prefix := parts[0]
	suffix := parts[1]

	var result []string

	// Always include the original
	result = append(result, code)

	// If suffix is not empty, add version without suffix
	if suffix != "" {
		result = append(result, prefix+"+")
	}

	// Generate padded versions by progressively removing precision
	// from the suffix, then from the prefix

	// Start from the full suffix and work backwards
	if len(suffix) == 2 {
		// Add version with first char of suffix only
		result = append(result, prefix+"+"+string(suffix[0]))
	}

	// Now pad the prefix progressively
	// E.g., 6FG22222 → 6FG22200 → 6FG20000 → 6F000000 → 60000000 (if we go that far)

	// Pad last char of prefix (2 chars = 1 pair)
	if len(prefix) >= 2 {
		padded := prefix[:len(prefix)-2] + "00"
		result = append(result, padded+"+")
	}

	// Pad last 4 chars of prefix (2 pairs)
	if len(prefix) >= 4 {
		padded := prefix[:len(prefix)-4] + "0000"
		result = append(result, padded+"+")
	}

	// Pad last 6 chars of prefix (3 pairs)
	if len(prefix) >= 6 {
		padded := prefix[:len(prefix)-6] + "000000"
		result = append(result, padded+"+")
	}

	// Pad all 8 chars (4 pairs)
	result = append(result, "00000000+")

	return result
}

// CalculateProximity calculates a proximity score between two pluscodes
// Score is based on how many characters match from the start
// Returns 0-500: (matching_chars / 8) * 500
func CalculateProximity(location1, location2 string) int {
	if !ValidatePluscode(location1) || !ValidatePluscode(location2) {
		return 0
	}

	// Extract just the prefix (before the +)
	prefix1 := strings.Split(location1, "+")[0]
	prefix2 := strings.Split(location2, "+")[0]

	// Count matching characters from the start
	matchingChars := 0
	maxLen := 8

	for i := 0; i < maxLen; i++ {
		if i < len(prefix1) && i < len(prefix2) && prefix1[i] == prefix2[i] {
			matchingChars++
		} else {
			break
		}
	}

	// Score: (matching_chars / 8) * 500
	// Max 500 for exact match, min 0 for no match
	return (matchingChars * 500) / 8
}

// IsLocationMatch checks if a message location matches the filter location
// Returns true if the locations are close enough
// For now, we consider it a match if they share at least the first 6 chars (within city level)
func IsLocationMatch(messageLocation, filterLocation string) bool {
	if !ValidatePluscode(messageLocation) || !ValidatePluscode(filterLocation) {
		return false
	}

	prefix1 := strings.Split(messageLocation, "+")[0]
	prefix2 := strings.Split(filterLocation, "+")[0]

	// Check first 6 characters (city-level matching)
	if len(prefix1) >= 6 && len(prefix2) >= 6 {
		return prefix1[:6] == prefix2[:6]
	}

	// If either is shorter, do simple comparison
	minLen := len(prefix1)
	if len(prefix2) < minLen {
		minLen = len(prefix2)
	}

	if minLen == 0 {
		return false
	}

	return prefix1[:minLen] == prefix2[:minLen]
}

// AllPlustags extracts all plustags (both direct and from #geo hashtags)
// and returns them deduplicated
func AllPlustags(text string) []string {
	directPlustags := ExtractPluscodes(text)
	geoPlustags := ExtractGeoHashtags(text)

	seen := make(map[string]bool)
	var result []string

	for _, code := range directPlustags {
		if !seen[code] {
			seen[code] = true
			result = append(result, code)
		}
	}

	for _, code := range geoPlustags {
		if !seen[code] {
			seen[code] = true
			result = append(result, code)
		}
	}

	return result
}
