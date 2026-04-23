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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, WrapText(tt.text, tt.width))
		})
	}
}
