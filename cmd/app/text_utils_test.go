package main

import (
	"strings"
	"testing"
)

func TestCountGraphemes(t *testing.T) {
	wave := "Hello \U0001f44b World"
	family := "Family: \U0001f468\u200d\U0001f469\u200d\U0001f467\u200d\U0001f466"
	emojis := "\U0001f916\U0001f4bb\U0001f680"

	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"Simple ASCII", "Hello World", 11},
		{"With emoji", wave, 13},
		{"Complex emoji", family, 9},
		{"Multiple emojis", emojis, 3},
		{"Empty string", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := countGraphemes(tt.text)
			if result != tt.expected {
				t.Errorf("countGraphemes(%q) = %d; want %d", tt.text, result, tt.expected)
			}
		})
	}
}

func TestSplitTextIntoChunks(t *testing.T) {
	tests := []struct {
		name         string
		text         string
		maxGraphemes int
		minChunks    int
		maxChunks    int
	}{
		{
			name:         "Short text",
			text:         "This is a short message",
			maxGraphemes: 300,
			minChunks:    1,
			maxChunks:    1,
		},
		{
			name:         "Long text needs splitting",
			text:         strings.Repeat("a", 500),
			maxGraphemes: 100,
			minChunks:    2,
			maxChunks:    10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := splitTextIntoChunks(tt.text, tt.maxGraphemes)

			if len(chunks) < tt.minChunks || len(chunks) > tt.maxChunks {
				t.Errorf("splitTextIntoChunks returned %d chunks; want between %d and %d",
					len(chunks), tt.minChunks, tt.maxChunks)
			}

			for i, chunk := range chunks {
				graphemes := countGraphemes(chunk)
				if graphemes > tt.maxGraphemes {
					t.Errorf("Chunk %d has %d graphemes; max is %d", i, graphemes, tt.maxGraphemes)
				}
			}
		})
	}
}

func TestSplitTextRealWorld(t *testing.T) {
	longText := `This is a very long response that exceeds the 300 grapheme limit set by Bluesky.
We need to make sure this gets split into multiple posts correctly. The splitting should respect
word boundaries when possible, and each chunk should be properly marked with continuation indicators
like (1/3), (2/3), etc. This way users will know they're reading a thread of connected messages.
The implementation uses the uniseg library to properly count grapheme clusters, which is important
because some characters that look like one character are actually multiple Unicode code points.
Emojis like ` + "\U0001f468\u200d\U0001f469\u200d\U0001f467\u200d\U0001f466" + ` are a good example of this - they're composed of multiple code points but should
be counted as a single grapheme. This ensures we don't split in the middle of an emoji or other
multi-codepoint grapheme cluster.`

	chunks := splitTextIntoChunks(longText, 300)

	t.Logf("Split into %d chunks", len(chunks))
	for i, chunk := range chunks {
		graphemes := countGraphemes(chunk)
		t.Logf("Chunk %d: %d graphemes", i+1, graphemes)
		if graphemes > 300 {
			t.Errorf("Chunk %d exceeds 300 graphemes: %d", i+1, graphemes)
		}
	}

	if len(chunks) < 2 {
		t.Error("Expected long text to be split into multiple chunks")
	}
}
