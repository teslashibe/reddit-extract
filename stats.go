package redditextract

// ReadStats reports parsing and filtering outcomes when reading JSONL input.
type ReadStats struct {
	TotalLines  int            `json:"total_lines"`
	Parsed      int            `json:"parsed"`
	Skipped     int            `json:"skipped"`
	SkipReasons map[string]int `json:"skip_reasons"`
	Errors      int            `json:"errors"`
}

func (s *ReadStats) addSkip(reason string) {
	if reason == "" {
		return
	}
	s.Skipped++
	if s.SkipReasons == nil {
		s.SkipReasons = make(map[string]int)
	}
	s.SkipReasons[reason]++
}
