package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	redditextract "github.com/teslashibe/reddit-extract"
	anthropicprovider "github.com/teslashibe/reddit-extract/providers/anthropic"
	openaiprovider "github.com/teslashibe/reddit-extract/providers/openai"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	if strings.HasPrefix(os.Args[1], "-") {
		cmdRun(os.Args[1:])
		return
	}

	switch os.Args[1] {
	case "run":
		cmdRun(os.Args[2:])
	case "read":
		cmdRead(os.Args[2:])
	case "version":
		fmt.Printf("reddit-extract v%s\n", version)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`reddit-extract v%s — LLM-powered structured extraction from Reddit JSONL

Usage:
  reddit-extract run --input <file.jsonl> --schema <schema.json> --output <results.jsonl>
  reddit-extract --input <file.jsonl> --schema <schema.json> --output <results.jsonl>
  reddit-extract read --input <file.jsonl>
  reddit-extract version
`, version)
}

func cmdRead(args []string) {
	fs := flag.NewFlagSet("read", flag.ExitOnError)
	input := fs.String("input", "", "Input reddit-scraper JSONL file")
	sample := fs.Int("sample", 2, "Show N sample records")
	fs.Parse(args)

	if strings.TrimSpace(*input) == "" {
		fatal("--input is required")
	}

	reader := redditextract.NewReader()
	records, stats, err := reader.ReadFile(*input)
	if err != nil {
		fatal("read input: %v", err)
	}

	fmt.Printf("reddit-extract v%s\n\n", version)
	fmt.Printf("Input:       %s\n", *input)
	fmt.Printf("Total lines: %d\n", stats.TotalLines)
	fmt.Printf("Parsed:      %d\n", stats.Parsed)
	fmt.Printf("Skipped:     %d\n", stats.Skipped)
	fmt.Printf("Errors:      %d\n", stats.Errors)

	if len(stats.SkipReasons) > 0 {
		fmt.Println("\nSkip reasons:")
		for reason, count := range stats.SkipReasons {
			fmt.Printf("  %-15s %d\n", reason, count)
		}
	}

	if len(records) > 0 && *sample > 0 {
		n := *sample
		if n > len(records) {
			n = len(records)
		}
		fmt.Printf("\nSample records (%d):\n", n)
		for i := 0; i < n; i++ {
			record := records[i]
			fmt.Printf("  [%d] r/%s | %s\n", i+1, record.Subreddit, truncate(record.Title, 80))
			fmt.Printf("      comments=%d url=%s\n", len(record.Comments), record.SourceURL)
		}
	}
}

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	input := fs.String("input", "", "Input reddit-scraper JSONL file")
	schemaPath := fs.String("schema", "", "JSON schema file")
	output := fs.String("output", "", "Output JSONL file")

	provider := fs.String("provider", "anthropic", "LLM provider: anthropic|openai")
	model := fs.String("model", "", "Provider model override")

	anthropicKey := fs.String("anthropic-key", envOr("ANTHROPIC_API_KEY", ""), "Anthropic API key")
	openaiKey := fs.String("openai-key", envOr("OPENAI_API_KEY", ""), "OpenAI API key")

	batch := fs.Bool("batch", false, "Use provider batch mode when available")
	batchSize := fs.Int("batch-size", 1000, "Items per submitted batch")
	concurrency := fs.Int("concurrency", 5, "Real-time request concurrency")
	maxTokens := fs.Int("max-tokens", 2048, "Per-request max tokens")
	temperature := fs.Float64("temperature", 0.0, "Sampling temperature")

	systemPrompt := fs.String("system-prompt", "", "Custom system prompt")
	systemPromptFile := fs.String("system-prompt-file", "", "Path to system prompt text file")
	promptBodyLimit := fs.Int("prompt-body-limit", 8000, "Max body chars in user prompt")
	promptCommentLimit := fs.Int("prompt-comment-limit", 50, "Max comments in user prompt")
	verbose := fs.Bool("verbose", false, "Verbose logs")

	fs.Parse(args)

	if strings.TrimSpace(*input) == "" {
		fatal("--input is required")
	}
	if strings.TrimSpace(*schemaPath) == "" {
		fatal("--schema is required")
	}
	if strings.TrimSpace(*output) == "" {
		fatal("--output is required")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	reader := redditextract.NewReader()
	records, stats, err := reader.ReadFile(*input)
	if err != nil {
		fatal("read input: %v", err)
	}
	fmt.Printf("Read %d records (%d skipped, %d errors)\n", stats.Parsed, stats.Skipped, stats.Errors)
	if len(records) == 0 {
		fatal("no extractable records found")
	}

	schemaBytes, err := os.ReadFile(*schemaPath)
	if err != nil {
		fatal("read schema file: %v", err)
	}
	schema, err := redditextract.DynamicSchemaFromJSON(schemaBytes)
	if err != nil {
		fatal("parse schema file: %v", err)
	}

	finalSystemPrompt := strings.TrimSpace(*systemPrompt)
	if strings.TrimSpace(*systemPromptFile) != "" {
		promptBytes, err := os.ReadFile(*systemPromptFile)
		if err != nil {
			fatal("read system prompt file: %v", err)
		}
		finalSystemPrompt = strings.TrimSpace(string(promptBytes))
	}

	client := buildProviderClient(*provider, *model, *anthropicKey, *openaiKey)
	if *batch {
		if _, ok := client.(redditextract.BatchClient); !ok {
			fatal("provider %q does not support --batch mode", *provider)
		}
	}

	opts := []redditextract.ExtractorOption{
		redditextract.WithConcurrency(*concurrency),
		redditextract.WithBatchSize(*batchSize),
		redditextract.WithPromptBodyLimit(*promptBodyLimit),
		redditextract.WithPromptCommentLimit(*promptCommentLimit),
		redditextract.WithMaxTokens(*maxTokens),
		redditextract.WithTemperature(*temperature),
	}
	if finalSystemPrompt != "" {
		opts = append(opts, redditextract.WithSystemPrompt(finalSystemPrompt))
	}
	if *batch {
		opts = append(opts, redditextract.WithBatchMode())
	} else {
		opts = append(opts, redditextract.WithRealTimeMode())
	}
	if *verbose {
		opts = append(opts,
			redditextract.WithProgress(func(completed, total int) {
				fmt.Printf("progress: %d/%d\n", completed, total)
			}),
			redditextract.WithBatchProgress(func(jobID string, status redditextract.BatchStatus) {
				fmt.Printf("batch %s: %s (%d/%d)\n", jobID, status.State, status.Completed, status.Total)
			}),
		)
	}

	extractor := redditextract.New(client, opts...)

	start := time.Now()
	results, err := redditextract.RunDynamic(ctx, extractor, records, schema)
	if err != nil {
		fatal("run extraction: %v", err)
	}
	if err := redditextract.WriteJSONL(*output, results); err != nil {
		fatal("write output: %v", err)
	}

	success := 0
	failed := 0
	for _, result := range results {
		if result.Error == nil {
			success++
		} else {
			failed++
		}
	}

	fmt.Printf("Wrote %d results to %s\n", len(results), *output)
	fmt.Printf("Succeeded: %d | Failed: %d | Duration: %s\n", success, failed, time.Since(start).Round(time.Second))
}

func buildProviderClient(provider, model, anthropicKey, openaiKey string) redditextract.LLMClient {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "anthropic":
		if strings.TrimSpace(anthropicKey) == "" {
			fatal("--anthropic-key or ANTHROPIC_API_KEY is required when provider=anthropic")
		}
		opts := []anthropicprovider.Option{}
		if strings.TrimSpace(model) != "" {
			opts = append(opts, anthropicprovider.WithModel(model))
		}
		return anthropicprovider.New(anthropicKey, opts...)
	case "openai":
		if strings.TrimSpace(openaiKey) == "" {
			fatal("--openai-key or OPENAI_API_KEY is required when provider=openai")
		}
		opts := []openaiprovider.Option{}
		if strings.TrimSpace(model) != "" {
			opts = append(opts, openaiprovider.WithModel(model))
		}
		return openaiprovider.New(openaiKey, opts...)
	default:
		fatal("unknown --provider value %q (expected anthropic or openai)", provider)
		return nil
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
