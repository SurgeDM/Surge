package tui

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
)

// ApplyGradient applies a vertical gradient to a multi-line string.
func ApplyGradient(text string, startColor, endColor color.Color) string {
	lines := strings.Split(text, "\n")
	height := len(lines)
	if height == 0 {
		return text
	}

	startRGB := colorToRGB(startColor)
	endRGB := colorToRGB(endColor)

	var coloredLines []string
	for i, line := range lines {
		// Calculate interpolation factor t [0, 1]
		// If there is only one line, t will be 0 (startColor)
		t := 0.0
		if height > 1 {
			t = float64(i) / float64(height-1)
		}

		// Interpolate RGB values
		r := uint8(math.Round(lerp(float64(startRGB.r), float64(endRGB.r), t)))
		g := uint8(math.Round(lerp(float64(startRGB.g), float64(endRGB.g), t)))
		b := uint8(math.Round(lerp(float64(startRGB.b), float64(endRGB.b), t)))

		// Create color string
		hexColor := fmt.Sprintf("#%02x%02x%02x", r, g, b)
		color := lipgloss.Color(hexColor)

		// Apply style to the line
		// Preserving Bold(true) as in the original LogoStyle
		coloredLine := lipgloss.NewStyle().Foreground(color).Bold(true).Render(line)
		coloredLines = append(coloredLines, coloredLine)
	}

	return strings.Join(coloredLines, "\n")
}

type rgb struct {
	r, g, b uint8
}

func colorToRGB(c color.Color) rgb {
	if c == nil {
		return rgb{}
	}
	r, g, b, _ := c.RGBA()
	return rgb{
		r: uint8(r >> 8),
		g: uint8(g >> 8),
		b: uint8(b >> 8),
	}
}

func lerp(a, b, t float64) float64 {
	return a + (b-a)*t
}
