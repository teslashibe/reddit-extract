// Package redditextract provides LLM-powered structured extraction over
// reddit-scraper JSONL data.
//
// The package has three composable parts:
//   - Reader: parse and filter reddit-scraper JSONL into []ContentRecord
//   - Schema: generate JSON Schema from Go structs or build runtime schemas
//   - Extractor: run real-time or batch LLM extraction into typed results
//
// Provider adapters can be implemented for any LLM vendor by satisfying
// LLMClient (and optionally BatchClient). Reference implementations live in
// providers/anthropic and providers/openai.
package redditextract
