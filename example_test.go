package redditextract

import (
	"context"
	"fmt"
	"time"
)

type exampleClient struct {
	reply string
}

func (c exampleClient) Complete(_ context.Context, req CompletionRequest) (CompletionResponse, error) {
	return CompletionResponse{
		ID:      req.ID,
		Content: c.reply,
		Model:   "example-model",
	}, nil
}

type trendSummary struct {
	Trend string `json:"trend" desc:"Primary trend discussed"`
}

func ExampleRun() {
	client := exampleClient{
		reply: `{"trend":"sleep quality improvement"}`,
	}
	extractor := New(client)
	records := []ContentRecord{
		{
			ID:          "abc123",
			Source:      SourceReddit,
			Subreddit:   "whoop",
			Title:       "Better sleep with routines",
			Body:        "My sleep score improved after changing habits.",
			PublishedAt: time.Now().UTC(),
		},
	}

	results, err := Run[trendSummary](context.Background(), extractor, records)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(results[0].Data.Trend)
	// Output: sleep quality improvement
}

func ExampleRunDynamic() {
	client := exampleClient{
		reply: `{"sentiment":"positive","themes":["sleep","recovery"]}`,
	}
	extractor := New(client)
	records := []ContentRecord{
		{
			ID:          "abc123",
			Source:      SourceReddit,
			Subreddit:   "whoop",
			Title:       "Post title",
			Body:        "Post body",
			PublishedAt: time.Now().UTC(),
		},
	}

	schema := NewDynamicSchemaBuilder("Runtime").
		AddStringField("sentiment", "Overall sentiment", true, "positive", "neutral", "negative").
		AddArrayField("themes", "Themes", false, map[string]any{"type": "string"}).
		Build()

	results, err := RunDynamic(context.Background(), extractor, records, schema)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println(results[0].Data["sentiment"])
	// Output: positive
}
