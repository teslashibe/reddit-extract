package anthropic

import (
	"encoding/json"
	"testing"
)

func TestFormatAnthropicBatchError(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{
			raw:  `{"type":"error","error":{"type":"invalid_request_error","message":"model: invalid"}}`,
			want: "invalid_request_error: model: invalid",
		},
		{
			raw:  `{"type":"invalid_request_error","message":"bad request"}`,
			want: "invalid_request_error: bad request",
		},
		{"", "unknown batch error"},
	}
	for _, tc := range cases {
		var raw json.RawMessage
		if tc.raw != "" {
			raw = json.RawMessage(tc.raw)
		}
		got := formatAnthropicBatchError(raw)
		if got != tc.want {
			t.Errorf("formatAnthropicBatchError(%q) = %q; want %q", tc.raw, got, tc.want)
		}
	}
}
