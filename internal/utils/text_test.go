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
			assert.Equal(t, tt.expected, truncateMiddle(tt.text, tt.limit))
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
			assert.Equal(t, tt.expected, truncateMiddle(tt.text, tt.limit))
		})
	}
}
