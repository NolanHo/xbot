package llm

import (
	"regexp"
	"strings"
)

// Pre-compiled regexes for think/reasoning block extraction.
var (
	thinkBlockRe     = regexp.MustCompile(`(?s)<think\b[^>]*>(.*?)</think\s*>`)
	reasoningBlockRe = regexp.MustCompile(`(?s)<reasoning>(.*?)</reasoning>`)
	thinkingBlockRe  = regexp.MustCompile(`(?s)<thinking>(.*?)</thinking>`)
)

// ExtractThinkBlocks extracts thinking/reasoning content from text.
// Supports: <think...</think, <reasoning>...</reasoning>, <thinking>...</thinking>
// Also considers response.ReasoningContent from structured API fields.
// Returns the inner thinking content with tags stripped, or empty string.
func ExtractThinkBlocks(content string) string {
	if content == "" {
		return ""
	}
	var parts []string
	extract := func(re *regexp.Regexp) {
		matches := re.FindAllStringSubmatch(content, -1)
		for _, m := range matches {
			if len(m) > 1 && m[1] != "" {
				parts = append(parts, strings.TrimSpace(m[1]))
			}
		}
	}
	extract(thinkBlockRe)
	extract(reasoningBlockRe)
	extract(thinkingBlockRe)
	return strings.Join(parts, "\n")
}
