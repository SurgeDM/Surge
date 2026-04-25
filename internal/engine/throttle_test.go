package engine

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLimiter(t *testing.T) {
	rate := int64(100) // 100 bytes per second
	l := NewLimiter(rate)
	assert.NotNil(t, l)

	// Consume burst immediately
	err := l.waitN(context.Background(), int(l.burst))
	assert.NoError(t, err)

	// Try to consume more, should block
	start := time.Now()
	err = l.waitN(context.Background(), 50)
	assert.NoError(t, err)
	elapsed := time.Since(start)

	// Should have waited roughly 0.5s
	assert.True(t, elapsed >= 400*time.Millisecond, "Should have waited at least 400ms, got %v", elapsed)
}

func TestThrottledReader(t *testing.T) {
	data := make([]byte, 200)
	for i := range data {
		data[i] = byte(i)
	}
	r := bytes.NewReader(data)

	rate := int64(100) // 100 bytes per second
	l := NewLimiter(rate)

	tr := NewThrottledReader(context.Background(), r, l)
	assert.NotNil(t, tr)

	buf := make([]byte, 50)

	// First read consumes burst
	n, err := tr.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, 50, n)

	// Second read might also fit in burst depending on timing/burst size
	n, err = tr.Read(buf)
	assert.NoError(t, err)
	assert.Equal(t, 50, n)

	// Third read should eventually wait
	totalRead := 100
	for totalRead < 200 {
		n, err = tr.Read(buf)
		if err == io.EOF {
			break
		}
		assert.NoError(t, err)
		totalRead += n
	}
	// Total 200 bytes at 100 bytes/sec with burst of 200 might finish fast.
	// But we verify the logic works via TestLimiter and TestDualLimiter.
}

func TestDualLimiter(t *testing.T) {
	data := make([]byte, 1000)
	r := bytes.NewReader(data)

	l1 := NewLimiter(1000) // 1000 bytes/sec
	l2 := NewLimiter(100)  // 100 bytes/sec (tighter)

	tr := NewThrottledReader(context.Background(), r, l1, l2)

	start := time.Now()
	buf := make([]byte, 100)
	total := 0
	for total < 300 {
		n, err := tr.Read(buf)
		if err == io.EOF {
			break
		}
		t.Logf("Read %d bytes, total %d", n, total+n)
		total += n
	}
	// Should be limited by l2 (100 bytes/sec)
	// 300 bytes at 100 bytes/sec should take ~1s (minus burst)
	// Burst for l2 is 200. So 100 bytes need to be "earned".
	// That's 1s wait.
	assert.True(t, time.Since(start) >= 800*time.Millisecond, "Should be limited by tighter limiter, got %v", time.Since(start))
}
