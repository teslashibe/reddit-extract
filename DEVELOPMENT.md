# Development Guide

This document is for contributors implementing features or provider adapters in `reddit-extract`.

## Prerequisites

- Go 1.22+ (module currently uses Go 1.25.x in `go.mod`)
- Access to sample `reddit-scraper` JSONL files for local testing

## Local setup

```bash
git clone https://github.com/teslashibe/reddit-extract.git
cd reddit-extract
go test ./...
```

## Core architecture

### 1) Reader layer

- `post.go`: raw Reddit post/comment structs matching `reddit-scraper` output
- `reader.go`: parsing/filtering + comment flattening
- `record.go`: normalized `ContentRecord`

### 2) Schema layer

- `schema.go`: generate schema from typed structs OR build runtime schemas

### 3) Extraction layer

- `llm.go` + `batch.go`: provider contracts
- `prompt.go`: system/user prompt construction
- `parse.go`: robust JSON extraction from model outputs
- `extractor.go`: orchestration (real-time + batch)

### 4) Provider adapters

- `providers/anthropic`: real-time + batch implementation
- `providers/openai`: real-time implementation

### 5) Output layer

- `writer.go`: JSONL output persistence

## Common commands

Run tests:

```bash
go test ./...
```

Format:

```bash
gofmt -w *.go cmd/reddit-extract/*.go providers/anthropic/*.go providers/openai/*.go
```

Run CLI help:

```bash
go run ./cmd/reddit-extract help
```

Preview ingest:

```bash
go run ./cmd/reddit-extract read --input /path/to/r_subreddit_YYYY-MM-DD.jsonl
```

Run extraction:

```bash
go run ./cmd/reddit-extract run \
  --input /path/to/r_subreddit_YYYY-MM-DD.jsonl \
  --schema ./schema.json \
  --output ./output/results.jsonl \
  --provider anthropic
```

## Adding a new provider

1. Create `providers/<name>/client.go`
2. Implement `redditextract.LLMClient`
3. Optionally implement `redditextract.BatchClient`
4. Add CLI wiring in `cmd/reddit-extract/main.go` (`buildProviderClient`)
5. Add README provider notes

Provider checklist:

- [ ] Context-aware HTTP requests
- [ ] Structured error messages with HTTP status
- [ ] Fill `CompletionResponse.Model` and `Usage` when available
- [ ] Preserve request IDs for batch correlation

## Testing expectations

- Unit tests should not call external APIs.
- Use mocks/fakes for `LLMClient` and `BatchClient`.
- Keep tests deterministic and fast.

Current high-value test areas:

- Reader filtering and flattening behavior
- Schema generation for nested + optional types
- Prompt truncation/formatting
- Parse robustness for code-fenced / noisy outputs
- Extractor error isolation and retry behavior
- Batch orchestration semantics

## Documentation expectations

- Every exported symbol must have a godoc comment.
- Keep README examples current with real APIs.
- Add `Example*` tests for new externally-visible flows.
