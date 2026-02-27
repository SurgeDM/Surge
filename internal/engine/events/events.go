package events

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/surge-downloader/surge/internal/engine/types"
)

// ProgressMsg represents a progress update from the downloader
type ProgressMsg struct {
	DownloadID        string
	Downloaded        int64
	Total             int64
	Speed             float64 // bytes per second
	Elapsed           time.Duration
	ActiveConnections int
	ChunkBitmap       []byte
	BitmapWidth       int
	ActualChunkSize   int64
	ChunkProgress     []int64
}

// DownloadCompleteMsg signals that the download finished successfully
type DownloadCompleteMsg struct {
	DownloadID string
	Filename   string
	Elapsed    time.Duration
	Total      int64
	AvgSpeed   float64 // Average download speed in bytes/sec
}

// DownloadErrorMsg signals that an error occurred
type DownloadErrorMsg struct {
	DownloadID string
	Filename   string
	Err        error
}

func (m DownloadErrorMsg) MarshalJSON() ([]byte, error) {
	type encoded struct {
		DownloadID string `json:"DownloadID"`
		Filename   string `json:"Filename,omitempty"`
		Err        string `json:"Err,omitempty"`
	}

	out := encoded{
		DownloadID: m.DownloadID,
		Filename:   m.Filename,
	}
	if m.Err != nil {
		out.Err = m.Err.Error()
	}

	return json.Marshal(out)
}

func (m *DownloadErrorMsg) UnmarshalJSON(data []byte) error {
	var aux struct {
		DownloadID string          `json:"DownloadID"`
		Filename   string          `json:"Filename"`
		Err        json.RawMessage `json:"Err"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	m.DownloadID = aux.DownloadID
	m.Filename = aux.Filename
	m.Err = nil

	if len(aux.Err) == 0 {
		return nil
	}

	// Most common case: server sends Err as a string.
	var errStr string
	if err := json.Unmarshal(aux.Err, &errStr); err == nil {
		if errStr != "" {
			m.Err = errors.New(errStr)
		}
		return nil
	}

	// Backward/forward compatibility: accept non-string payloads (e.g. {}).
	raw := string(aux.Err)
	if raw != "" && raw != "null" {
		m.Err = errors.New(raw)
	}
	return nil
}

// DownloadStartedMsg is sent when a download actually starts (after metadata fetch)
type DownloadStartedMsg struct {
	DownloadID string
	URL        string
	Filename   string
	Total      int64
	DestPath   string               // Full path to the destination file
	State      *types.ProgressState `json:"-"`
}

type DownloadPausedMsg struct {
	DownloadID string
	Filename   string
	Downloaded int64
}

type DownloadResumedMsg struct {
	DownloadID string
	Filename   string
}

type DownloadQueuedMsg struct {
	DownloadID string
	Filename   string
	URL        string
	DestPath   string
}

type DownloadRemovedMsg struct {
	DownloadID string
	Filename   string
}

// SystemLogMsg carries informational system-level log messages for clients/UI.
type SystemLogMsg struct {
	Message string
}

// BatchProgressMsg represents a batch of progress updates to reduce TUI render calls
type BatchProgressMsg []ProgressMsg

// DownloadRequestMsg signals a request to start a download (e.g. from extension)
// that may need user confirmation or duplicate checking
type DownloadRequestMsg struct {
	ID       string
	URL      string
	Filename string
	Path     string
	Mirrors  []string
	Headers  map[string]string
}

const (
	EventTypeProgress = "progress"
	EventTypeStarted  = "started"
	EventTypeComplete = "complete"
	EventTypeError    = "error"
	EventTypePaused   = "paused"
	EventTypeResumed  = "resumed"
	EventTypeQueued   = "queued"
	EventTypeRemoved  = "removed"
	EventTypeRequest  = "request"
	EventTypeSystem   = "system"
)

// SSEMessage represents one server-sent event frame.
type SSEMessage struct {
	Event string
	Data  []byte
}

// EncodeSSEMessages converts an event payload into one or more SSE messages.
// BatchProgressMsg is flattened into multiple "progress" events.
func EncodeSSEMessages(msg interface{}) ([]SSEMessage, error) {
	switch m := msg.(type) {
	case BatchProgressMsg:
		frames := make([]SSEMessage, 0, len(m))
		for _, p := range m {
			data, err := json.Marshal(p)
			if err != nil {
				return nil, err
			}
			frames = append(frames, SSEMessage{
				Event: EventTypeProgress,
				Data:  data,
			})
		}
		return frames, nil
	default:
		eventType, ok := EventTypeForMessage(msg)
		if !ok {
			return nil, nil
		}
		data, err := json.Marshal(msg)
		if err != nil {
			return nil, err
		}
		return []SSEMessage{{
			Event: eventType,
			Data:  data,
		}}, nil
	}
}

// EventTypeForMessage maps message payloads to SSE event type names.
func EventTypeForMessage(msg interface{}) (string, bool) {
	switch msg.(type) {
	case ProgressMsg:
		return EventTypeProgress, true
	case DownloadStartedMsg:
		return EventTypeStarted, true
	case DownloadCompleteMsg:
		return EventTypeComplete, true
	case DownloadErrorMsg:
		return EventTypeError, true
	case DownloadPausedMsg:
		return EventTypePaused, true
	case DownloadResumedMsg:
		return EventTypeResumed, true
	case DownloadQueuedMsg:
		return EventTypeQueued, true
	case DownloadRemovedMsg:
		return EventTypeRemoved, true
	case DownloadRequestMsg:
		return EventTypeRequest, true
	case SystemLogMsg:
		return EventTypeSystem, true
	default:
		return "", false
	}
}

// DecodeSSEMessage decodes one SSE event payload into the corresponding message.
func DecodeSSEMessage(eventType string, data []byte) (interface{}, bool, error) {
	var msg interface{}

	switch eventType {
	case EventTypeProgress:
		var m ProgressMsg
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, true, err
		}
		msg = m
	case EventTypeStarted:
		var m DownloadStartedMsg
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, true, err
		}
		msg = m
	case EventTypeComplete:
		var m DownloadCompleteMsg
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, true, err
		}
		msg = m
	case EventTypeError:
		var m DownloadErrorMsg
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, true, err
		}
		msg = m
	case EventTypePaused:
		var m DownloadPausedMsg
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, true, err
		}
		msg = m
	case EventTypeResumed:
		var m DownloadResumedMsg
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, true, err
		}
		msg = m
	case EventTypeQueued:
		var m DownloadQueuedMsg
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, true, err
		}
		msg = m
	case EventTypeRemoved:
		var m DownloadRemovedMsg
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, true, err
		}
		msg = m
	case EventTypeRequest:
		var m DownloadRequestMsg
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, true, err
		}
		msg = m
	case EventTypeSystem:
		var m SystemLogMsg
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, true, err
		}
		msg = m
	default:
		return nil, false, nil
	}

	return msg, true, nil
}
