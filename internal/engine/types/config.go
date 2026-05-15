package types

import "time"

const (
	KB = 1 << 10
	MB = 1 << 20
	GB = 1 << 30
)

const (
	IncompleteSuffix = ".surge"

	MinChunk     = 2 * MB
	AlignSize    = 4 * KB
	WorkerBuffer = 512 * KB

	WorkerBatchSize     = 1 * MB
	WorkerBatchInterval = 200 * time.Millisecond

	PerDownloadMax = 32
	DialHedgeCount = 4

	DefaultMaxIdleConns          = 100
	DefaultIdleConnTimeout       = 90 * time.Second
	DefaultTLSHandshakeTimeout   = 10 * time.Second
	DefaultResponseHeaderTimeout = 15 * time.Second
	DefaultExpectContinueTimeout = 1 * time.Second
	DialTimeout                  = 10 * time.Second
	KeepAliveDuration            = 30 * time.Second
	ProbeTimeout                 = 30 * time.Second

	PoolMaxIdleConns        = 512
	PoolMaxIdleConnsPerHost = 128
	PoolMaxConnsPerHost     = 512

	MaxTaskRetries = 3
	RetryBaseDelay = 200 * time.Millisecond

	HealthCheckInterval = 1 * time.Second
	SlowWorkerThreshold = 0.30
	SlowWorkerGrace     = 5 * time.Second
	StallTimeout        = 3 * time.Second
	SpeedEMAAlpha       = 0.3

	ProgressChannelBuffer = 100
)

type DownloadConfig struct {
	URL        string
	OutputPath string
	DestPath   string
	ID         string
	Filename   string
	IsResume   bool
	ProgressCh chan<- any
	State      *ProgressState
	SavedState *DownloadState
	Runtime    *RuntimeConfig
	Mirrors    []string
	Headers    map[string]string

	IsExplicitCategory bool
	TotalSize          int64
	SupportsRange      bool
}

// RuntimeConfig carries network and downloader tuning knobs.
// Fields used by the downloader getters fall into two groups:
// zero means "use package default" for capacity-style settings such as
// connections, chunk size, buffer size, and retries; zero is preserved for
// opt-out settings where disabling a behavior is meaningful.
type RuntimeConfig struct {
	MaxConnectionsPerDownload int
	UserAgent                 string
	ProxyURL                  string
	CustomDNS                 string
	SequentialDownload        bool
	MinChunkSize              int64

	WorkerBufferSize      int
	MaxTaskRetries        int
	DialHedgeCount        int
	SlowWorkerThreshold   float64
	SlowWorkerGracePeriod time.Duration
	StallTimeout          time.Duration
	SpeedEmaAlpha         float64
}

const DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

func (r *RuntimeConfig) GetUserAgent() string {
	if r == nil || r.UserAgent == "" {
		return DefaultUserAgent
	}
	return r.UserAgent
}

func (r *RuntimeConfig) GetMaxConnectionsPerDownload() int {
	if r == nil || r.MaxConnectionsPerDownload <= 0 {
		return PerDownloadMax
	}
	return r.MaxConnectionsPerDownload
}

func (r *RuntimeConfig) GetMinChunkSize() int64 {
	if r == nil || r.MinChunkSize <= 0 {
		return MinChunk
	}
	return r.MinChunkSize
}

func (r *RuntimeConfig) GetWorkerBufferSize() int {
	if r == nil || r.WorkerBufferSize <= 0 {
		return WorkerBuffer
	}
	return r.WorkerBufferSize
}

func (r *RuntimeConfig) GetMaxTaskRetries() int {
	if r == nil || r.MaxTaskRetries <= 0 {
		return MaxTaskRetries
	}
	return r.MaxTaskRetries
}

func (r *RuntimeConfig) GetDialHedgeCount() int {
	if r == nil || r.DialHedgeCount < 0 {
		return DialHedgeCount
	}
	return r.DialHedgeCount
}

func (r *RuntimeConfig) GetSlowWorkerThreshold() float64 {
	if r == nil || r.SlowWorkerThreshold < 0 || r.SlowWorkerThreshold > 1 {
		return SlowWorkerThreshold
	}
	return r.SlowWorkerThreshold
}

func (r *RuntimeConfig) GetSlowWorkerGracePeriod() time.Duration {
	if r == nil || r.SlowWorkerGracePeriod < 0 {
		return SlowWorkerGrace
	}
	return r.SlowWorkerGracePeriod
}

func (r *RuntimeConfig) GetStallTimeout() time.Duration {
	if r == nil || r.StallTimeout < 0 {
		return StallTimeout
	}
	return r.StallTimeout
}

func (r *RuntimeConfig) GetSpeedEmaAlpha() float64 {
	if r == nil || r.SpeedEmaAlpha < 0 || r.SpeedEmaAlpha > 1 {
		return SpeedEMAAlpha
	}
	return r.SpeedEmaAlpha
}

// DefaultRuntimeConfig returns a fully-populated runtime config for callers
// that want engine defaults rather than relying on zero-value semantics.
func DefaultRuntimeConfig() *RuntimeConfig {
	return &RuntimeConfig{
		MaxConnectionsPerDownload: PerDownloadMax,
		UserAgent:                 DefaultUserAgent,
		ProxyURL:                  "",
		CustomDNS:                 "",
		SequentialDownload:        false,
		MinChunkSize:              MinChunk,
		WorkerBufferSize:          WorkerBuffer,
		MaxTaskRetries:            MaxTaskRetries,
		DialHedgeCount:            DialHedgeCount,
		SlowWorkerThreshold:       SlowWorkerThreshold,
		SlowWorkerGracePeriod:     SlowWorkerGrace,
		StallTimeout:              StallTimeout,
		SpeedEmaAlpha:             SpeedEMAAlpha,
	}
}
