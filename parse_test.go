package redditextract

import "testing"

type parseFixture struct {
	Topic string `json:"topic"`
	Score int    `json:"score"`
}

func TestParseResponsePlainJSON(t *testing.T) {
	got, err := ParseResponse[parseFixture](`{"topic":"recovery","score":7}`)
	if err != nil {
		t.Fatalf("ParseResponse() error = %v", err)
	}
	if got.Topic != "recovery" || got.Score != 7 {
		t.Fatalf("parsed mismatch: %+v", got)
	}
}

func TestParseResponseMarkdownFence(t *testing.T) {
	raw := "```json\n{\"topic\":\"sleep\",\"score\":4}\n```"
	got, err := ParseResponse[parseFixture](raw)
	if err != nil {
		t.Fatalf("ParseResponse() error = %v", err)
	}
	if got.Topic != "sleep" || got.Score != 4 {
		t.Fatalf("parsed mismatch: %+v", got)
	}
}

func TestParseResponseWithPreamble(t *testing.T) {
	raw := "Here is the JSON:\n{\"topic\":\"energy\",\"score\":9}\nThanks."
	got, err := ParseResponse[parseFixture](raw)
	if err != nil {
		t.Fatalf("ParseResponse() error = %v", err)
	}
	if got.Topic != "energy" || got.Score != 9 {
		t.Fatalf("parsed mismatch: %+v", got)
	}
}

func TestParseResponseErrorsOnInvalidJSON(t *testing.T) {
	_, err := ParseResponse[parseFixture]("not json")
	if err == nil {
		t.Fatal("expected error for invalid json")
	}
}
