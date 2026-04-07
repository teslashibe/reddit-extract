package redditextract

import (
	"encoding/json"
	"fmt"
	"strings"
)

// DefaultSystemPrompt is the baseline instruction set used when callers do not
// provide a custom system prompt.
const DefaultSystemPrompt = `You are a structured data extraction engine.

You receive one Reddit post (title, body, metadata, and comments) and a JSON schema.
Return exactly one JSON object that matches the schema.

Rules:
1. Output valid JSON only (no markdown, no prose).
2. Do not invent facts. If unknown, return null or empty arrays/objects as appropriate.
3. Keep extracted values concise and grounded in the provided text.
4. Respect enum constraints in the schema.`

// BuildSystemPrompt combines a base prompt with the target JSON schema.
func BuildSystemPrompt(basePrompt string, schema DynamicSchema) string {
	basePrompt = strings.TrimSpace(basePrompt)
	if basePrompt == "" {
		basePrompt = DefaultSystemPrompt
	}

	schemaJSON, err := json.MarshalIndent(schema.JSONSchema, "", "  ")
	if err != nil {
		schemaJSON = []byte(`{"type":"object"}`)
	}

	return fmt.Sprintf("%s\n\nJSON Schema:\n%s", basePrompt, string(schemaJSON))
}

// BuildUserPrompt renders one content record into a prompt-safe text block.
func BuildUserPrompt(record ContentRecord, maxBodyChars, maxComments int) string {
	if maxBodyChars <= 0 {
		maxBodyChars = 8000
	}
	if maxComments < 0 {
		maxComments = 50
	}

	var b strings.Builder
	b.WriteString("Extract structured data from this Reddit post.\n\n")
	b.WriteString(fmt.Sprintf("Source: %s\n", record.Source))
	b.WriteString(fmt.Sprintf("Subreddit: r/%s\n", record.Subreddit))
	b.WriteString(fmt.Sprintf("Post ID: %s\n", record.ID))
	if record.SourceURL != "" {
		b.WriteString(fmt.Sprintf("URL: %s\n", record.SourceURL))
	}
	b.WriteString(fmt.Sprintf("Published: %s\n", record.PublishedAt.Format("2006-01-02")))
	b.WriteString(fmt.Sprintf("Title: %s\n", record.Title))

	if len(record.Metadata) > 0 {
		meta, err := json.Marshal(record.Metadata)
		if err == nil {
			b.WriteString(fmt.Sprintf("Metadata: %s\n", string(meta)))
		}
	}

	b.WriteString("\nBody:\n")
	body := strings.TrimSpace(record.Body)
	if len(body) > maxBodyChars {
		body = body[:maxBodyChars] + "\n[...truncated]"
	}
	if body == "" {
		b.WriteString("(empty)\n")
	} else {
		b.WriteString(body)
		b.WriteString("\n")
	}

	if len(record.Comments) > 0 {
		b.WriteString("\nComments:\n")
		limit := len(record.Comments)
		if limit > maxComments {
			limit = maxComments
		}
		for i := 0; i < limit; i++ {
			comment := record.Comments[i]
			indent := strings.Repeat("  ", comment.Depth)
			opSuffix := ""
			if comment.IsSubmitter {
				opSuffix = " [OP]"
			}
			b.WriteString(fmt.Sprintf("%s[%d] %s%s (score: %d)\n", indent, i+1, comment.Author, opSuffix, comment.Score))
			b.WriteString(fmt.Sprintf("%s%s\n\n", indent, comment.Body))
		}
		if len(record.Comments) > maxComments {
			b.WriteString(fmt.Sprintf("[...%d more comments omitted]\n", len(record.Comments)-maxComments))
		}
	}

	return b.String()
}
