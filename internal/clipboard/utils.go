package clipboard

import (
	"github.com/atotto/clipboard"
)

var clipboardReadAll = clipboard.ReadAll
var clipboardWriteAll = clipboard.WriteAll

// Read returns the current text content of the system clipboard.
func Read() (string, error) {
	return clipboardReadAll()
}

// Write copies the given text to the system clipboard.
func Write(text string) error {
	return clipboardWriteAll(text)
}
