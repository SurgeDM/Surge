package utils

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestWrapText(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		width    int
		expected string
	}{
		{
			name:     "Simple wrap",
			text:     "hello world",
			width:    5,
			expected: "hello\nworld",
		},
		{
			name:     "No wrap needed",
			text:     "hello",
			width:    10,
			expected: "hello",
		},
		{
			name:     "Hard wrap long word",
			text:     "supercalifragilisticexpialidocious",
			width:    10,
			expected: "supercalif\nragilistic\nexpialidoc\nious",
		},
		{
			name:     "Wrap with multiple spaces",
			text:     "hello   world",
			width:    5,
			expected: "hello\nworld",
		},
		{
			name:     "Wrap with existing newlines",
			text:     "hello\nworld",
			width:    10,
			expected: "hello\nworld",
		},
		{
			name:     "Empty string",
			text:     "",
			width:    10,
			expected: "",
		},
		{
			name:     "Zero width",
			text:     "hello",
			width:    0,
			expected: "hello",
		},
		{
			name:     "Multi-byte runes (emojis)",
			text:     "🌟🌟🌟🌟🌟",
			width:    4, // Each emoji is width 2
			expected: "🌟🌟\n🌟🌟\n🌟",
		},
		{
			name:     "CJK characters",
			text:     "你好世界",
			width:    4, // Each character is width 2
			expected: "你好\n世界",
		},
		{
			name:     "Mixed ASCII and runes",
			text:     "hello 🌟 world",
			width:    8,
			expected: "hello 🌟\nworld",
		},
		{
			name:     "Hard wrap mid-sentence",
			text:     "short supercalifragilisticexpialidocious",
			width:    10,
			expected: "short\nsupercalif\nragilistic\nexpialidoc\nious",
		},
		{
			name:     "Width 1",
			text:     "abc",
			width:    1,
			expected: "a\nb\nc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, WrapText(tt.text, tt.width))
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		limit    int
		expected string
	}{
		{"ASCII", "hello world", 5, "hell…"},
		{"Emoji", "🌟🌟🌟", 4, "🌟…"}, // 🌟 is width 2, so 🌟 is 2, next 🌟 would make it 4, but limit-1 is 3. So only one 🌟 fits.
		{"CJK", "你好世界", 5, "你好…"}, // 你是2, 好的2, 总共4. 世是2, 总共6 > 5. 所以只有你好.
		{"Limit 1", "hello", 1, "…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, Truncate(tt.text, tt.limit))
		})
	}
}

func TestTruncateMiddle(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		limit    int
		expected string
	}{
		{"ASCII", "1234567890", 5, "12…90"},
		{"Mixed", "abc🌟def", 6, "ab…def"}, // abc(3) 🌟(2) def(3). limit 6.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, TruncateMiddle(tt.text, tt.limit))
		})
	}
}

func TestTruncateMiddleEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		limit    int
		expected string
	}{
		{"Limit 1", "hello", 1, "…"},
		{"Limit 2", "hello", 2, "h…"},
		{"Limit 3", "hello", 3, "h…o"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, TruncateMiddle(tt.text, tt.limit))
		})
	}
}

func TestTruncateTwoLines(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		width    int
		expected string
	}{
		{"Single Line", "abc", 10, "abc"},
		{"Exactly Two Lines", "abcdefghij", 5, "abcde\nfghij"},
		{"With Spaces", "abc def", 4, "abc \ndef"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, TruncateTwoLines(tt.text, tt.width))
		})
	}
}

func TestAnsiAwareness(t *testing.T) {
	red := "\x1b[31m"
	reset := "\x1b[0m"
	text := red + "hello" + reset // visual width 5

	t.Run("truncateToWidth", func(t *testing.T) {
		assert.Equal(t, red+"hel"+reset, truncateToWidth(text, 3))
	})

	t.Run("TruncateMiddle", func(t *testing.T) {
		// limit 4: left 1, right 2
		assert.Equal(t, red+"h"+reset+"…"+red+"lo"+reset, TruncateMiddle(text, 4))
	})

	t.Run("Truncate ANSI Guard", func(t *testing.T) {
		// This should not be truncated because visual width is 5
		assert.Equal(t, text, Truncate(text, 10))
		// This should be truncated
		assert.Equal(t, red+"hel"+reset+"…", Truncate(text, 4))
	})

	t.Run("TruncateTwoLines ANSI carry-over", func(t *testing.T) {
		// width 3: first line "hel", second line should have "lo" with red color
		// Expected: red+"hel"+reset + "\n" + red+"lo"+reset
		expected := red + "hel" + reset + "\n" + red + "lo" + reset
		assert.Equal(t, expected, TruncateTwoLines(text, 3))
	})
}
