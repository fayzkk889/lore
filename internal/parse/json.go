// Package parse provides shared JSON cleaning utilities for API response parsing.
package parse

import (
	"regexp"
	"strings"
)

// fenceRe matches markdown code fences: ```json ... ``` or ``` ... ```
var fenceRe = regexp.MustCompile("(?s)```(?:json)?\\s*\\n?(.*?)\\n?```")

// trailingCommaRe matches trailing commas before } or ]
// e.g.  , }  or  ,\n]
var trailingCommaRe = regexp.MustCompile(`,\s*([\}\]])`)

// CleanJSON normalises a raw API response string into a bare JSON object.
//
// The cleaning pipeline is:
//  1. If an XML tag match is found (passed in via xmlContent), use that.
//  2. Otherwise look for ```json ... ``` or ``` ... ``` fences.
//  3. Strip any text before the first '{' and after the last '}'.
//  4. Remove trailing commas before '}' or ']'.
//  5. Trim whitespace.
//
// The xmlContent parameter should be the already-extracted content from between
// XML tags (empty string if no tags were found in the caller).
func CleanJSON(xmlContent, raw string) string {
	var s string

	if strings.TrimSpace(xmlContent) != "" {
		// Caller already extracted the XML tag content — use it directly.
		s = xmlContent
	} else {
		// Try markdown code fence extraction.
		if m := fenceRe.FindStringSubmatch(raw); m != nil {
			s = m[1]
		} else {
			s = raw
		}
	}

	// Strip text before first '{' and after last '}'.
	if i := strings.Index(s, "{"); i >= 0 {
		s = s[i:]
	}
	if i := strings.LastIndex(s, "}"); i >= 0 {
		s = s[:i+1]
	}

	// Remove trailing commas before } or ].
	s = trailingCommaRe.ReplaceAllString(s, "$1")

	return strings.TrimSpace(s)
}
