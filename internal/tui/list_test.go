package tui

import (
	"bytes"
	"testing"
	"github.com/charmbracelet/bubbles/list"
)

func BenchmarkDownloadDelegateRender(b *testing.B) {
	d := newDownloadDelegate()
	m := list.New([]list.Item{}, d, 100, 100)
	
	// mock download logic 
	di := DownloadItem{
		download: &DownloadModel{
			ID: "123",
			Filename: "ubuntu-22.04.iso",
			Total: 1024 * 1024 * 1000,
			Downloaded: 1024 * 1024 * 500,
			Speed: 10 * 1024 * 1024,
		},
	}
	
	var buf bytes.Buffer
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		d.Render(&buf, m, 0, di)
	}
}
