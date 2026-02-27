package events

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestEventTypeForMessage(t *testing.T) {
	tests := []struct {
		name      string
		msg       interface{}
		wantType  string
		wantFound bool
	}{
		{name: "progress", msg: ProgressMsg{}, wantType: EventTypeProgress, wantFound: true},
		{name: "started", msg: DownloadStartedMsg{}, wantType: EventTypeStarted, wantFound: true},
		{name: "complete", msg: DownloadCompleteMsg{}, wantType: EventTypeComplete, wantFound: true},
		{name: "error", msg: DownloadErrorMsg{}, wantType: EventTypeError, wantFound: true},
		{name: "paused", msg: DownloadPausedMsg{}, wantType: EventTypePaused, wantFound: true},
		{name: "resumed", msg: DownloadResumedMsg{}, wantType: EventTypeResumed, wantFound: true},
		{name: "queued", msg: DownloadQueuedMsg{}, wantType: EventTypeQueued, wantFound: true},
		{name: "removed", msg: DownloadRemovedMsg{}, wantType: EventTypeRemoved, wantFound: true},
		{name: "request", msg: DownloadRequestMsg{}, wantType: EventTypeRequest, wantFound: true},
		{name: "system", msg: SystemLogMsg{}, wantType: EventTypeSystem, wantFound: true},
		{name: "unknown", msg: struct{}{}, wantType: "", wantFound: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotFound := EventTypeForMessage(tt.msg)
			if gotType != tt.wantType || gotFound != tt.wantFound {
				t.Fatalf("EventTypeForMessage(%T) = (%q, %v), want (%q, %v)", tt.msg, gotType, gotFound, tt.wantType, tt.wantFound)
			}
		})
	}
}

func TestEncodeSSEMessages_BatchProgress(t *testing.T) {
	batch := BatchProgressMsg{
		{DownloadID: "a", Downloaded: 1, Total: 10},
		{DownloadID: "b", Downloaded: 2, Total: 10},
	}

	frames, err := EncodeSSEMessages(batch)
	if err != nil {
		t.Fatalf("EncodeSSEMessages(batch) failed: %v", err)
	}
	if len(frames) != 2 {
		t.Fatalf("EncodeSSEMessages(batch) produced %d frames, want 2", len(frames))
	}

	for i, frame := range frames {
		if frame.Event != EventTypeProgress {
			t.Fatalf("frame[%d] event = %q, want %q", i, frame.Event, EventTypeProgress)
		}
		var decoded ProgressMsg
		if err := json.Unmarshal(frame.Data, &decoded); err != nil {
			t.Fatalf("frame[%d] data failed to decode: %v", i, err)
		}
		if decoded.DownloadID != batch[i].DownloadID {
			t.Fatalf("frame[%d] decoded ID = %q, want %q", i, decoded.DownloadID, batch[i].DownloadID)
		}
	}
}

func TestEncodeDecodeSSEMessage_RoundTrip(t *testing.T) {
	original := DownloadErrorMsg{
		DownloadID: "dl-1",
		Filename:   "file.bin",
		Err:        errors.New("boom"),
	}

	frames, err := EncodeSSEMessages(original)
	if err != nil {
		t.Fatalf("EncodeSSEMessages failed: %v", err)
	}
	if len(frames) != 1 {
		t.Fatalf("EncodeSSEMessages produced %d frames, want 1", len(frames))
	}

	decoded, ok, err := DecodeSSEMessage(frames[0].Event, frames[0].Data)
	if err != nil {
		t.Fatalf("DecodeSSEMessage failed: %v", err)
	}
	if !ok {
		t.Fatal("DecodeSSEMessage reported unknown event for known frame")
	}

	msg, castOK := decoded.(DownloadErrorMsg)
	if !castOK {
		t.Fatalf("decoded message type = %T, want DownloadErrorMsg", decoded)
	}
	if msg.DownloadID != original.DownloadID || msg.Filename != original.Filename {
		t.Fatalf("decoded message mismatch: got %+v want %+v", msg, original)
	}
	if msg.Err == nil || msg.Err.Error() != "boom" {
		t.Fatalf("decoded error mismatch: got %v", msg.Err)
	}
}

func TestDecodeSSEMessage_UnknownType(t *testing.T) {
	decoded, ok, err := DecodeSSEMessage("not-a-real-event", []byte(`{"x":1}`))
	if err != nil {
		t.Fatalf("DecodeSSEMessage returned unexpected error: %v", err)
	}
	if ok {
		t.Fatalf("DecodeSSEMessage reported known type for unknown event: %+v", decoded)
	}
	if decoded != nil {
		t.Fatalf("DecodeSSEMessage decoded unknown event payload: %+v", decoded)
	}
}
