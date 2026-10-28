package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/SurgeDM/Surge/internal/clioutput"
	"github.com/SurgeDM/Surge/internal/service"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

// State changes invalidate cached snapshots; their payload never asserts a
// post-finalization state. This avoids regressing the initial snapshot when an
// event was buffered before it. Sequences are scoped to one stream epoch.
type agentEvent struct {
	SchemaVersion int                `json:"schema_version"`
	Type          string             `json:"type"`
	Epoch         string             `json:"epoch"`
	Sequence      uint64             `json:"sequence"`
	Time          string             `json:"time"`
	ID            string             `json:"id,omitempty"`
	Items         []downloadItem     `json:"items,omitempty"`
	Error         *clioutput.Failure `json:"error,omitempty"`
}

func validateEventIDs(ids []string) error {
	if len(ids) == 0 || len(ids) > maxWaitIDs {
		return usageError("Events requires 1-256 full download IDs.")
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if _, err := uuid.Parse(id); err != nil || len(id) != 36 || seen[id] {
			return usageError("Events requires unique full UUID download IDs.")
		}
		seen[id] = true
	}
	return nil
}
func agentEventsHandler(owner service.DownloadService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ids := strings.Split(r.URL.Query().Get("ids"), ",")
		interval := time.Second
		if raw := r.URL.Query().Get("interval_ms"); raw != "" {
			millis, err := strconv.Atoi(raw)
			if err != nil || millis < 250 || millis > 60000 {
				http.Error(w, "interval_ms must be 250-60000", http.StatusBadRequest)
				return
			}
			interval = time.Duration(millis) * time.Millisecond
		}
		if err := validateEventIDs(ids); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		observer, ok := owner.(service.ObserverService)
		if !ok {
			http.Error(w, "Filtered events unavailable", http.StatusNotImplemented)
			return
		}
		progress := r.URL.Query().Get("progress") == "true"
		stream, cleanup, err := observer.StreamObserver(r.Context(), types.ObserverOptions{IDs: ids, Progress: progress})
		if err != nil {
			http.Error(w, "Events unavailable", http.StatusServiceUnavailable)
			return
		}
		defer cleanup()
		// Subscribe first, then read snapshots; queued state_changed records are invalidations.
		statuses := make([]types.DownloadStatus, 0, len(ids))
		for _, id := range ids {
			status, err := owner.GetStatus(id)
			if err != nil || status == nil {
				http.Error(w, "A selected download was not found", http.StatusNotFound)
				return
			}
			statuses = append(statuses, *status)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		controller := http.NewResponseController(w)
		defer func() { _ = controller.SetWriteDeadline(time.Time{}) }()
		epoch := uuid.NewString()
		var sequence uint64
		send := func(event agentEvent) error {
			sequence++
			event.SchemaVersion = 1
			event.Epoch = epoch
			event.Sequence = sequence
			event.Time = time.Now().UTC().Format(time.RFC3339Nano)
			if err := prepareSSEWrite(controller); err != nil {
				return err
			}
			if err := json.NewEncoder(w).Encode(event); err != nil {
				return err
			}
			return flushSSEWrite(controller)
		}
		if err := send(agentEvent{Type: "snapshot", Items: agentDownloadItems(statuses)}); err != nil {
			return
		}
		lastProgress := make(map[string]time.Time, len(ids))
		for {
			select {
			case <-r.Context().Done():
				return
			case event, open := <-stream:
				if !open || event.ResyncRequired {
					_ = send(agentEvent{Type: "resync_required"})
					return
				}
				events := []types.DownloadEvent{event}
				if event.Type == types.EventBatchProgress {
					events = event.BatchEvents
				}
				for _, item := range events {
					notice := agentEvent{Type: "state_changed", ID: item.DownloadID}
					if item.Type == types.EventProgress {
						if !progress || time.Since(lastProgress[item.DownloadID]) < interval {
							continue
						}
						lastProgress[item.DownloadID] = time.Now()
						notice.Type = "progress"
						notice.Items = agentDownloadItems([]types.DownloadStatus{{ID: item.DownloadID, Status: "downloading", Phase: item.Phase, Downloaded: item.Downloaded, TotalSize: item.Total, Speed: item.Speed, Connections: item.Connections}})
					}
					if err := send(notice); err != nil {
						return
					}
				}
			}
		}
	}
}
func newEventsCommand() *cobra.Command {
	command := &cobra.Command{Use: "events ID...", Short: "Stream filtered NDJSON snapshots and invalidation notices", Annotations: map[string]string{"surge.contract": "1"}, Args: func(_ *cobra.Command, args []string) error { return validateEventIDs(args) }, RunE: func(cmd *cobra.Command, args []string) error {
		interval, _ := cmd.Flags().GetDuration("interval")
		timeout, _ := cmd.Flags().GetDuration("timeout")
		progress, _ := cmd.Flags().GetBool("progress")
		if interval < 250*time.Millisecond || interval > time.Minute || timeout < 0 {
			return usageError("--interval must be 250ms-1m and --timeout nonnegative.")
		}
		ctx := cmd.Context()
		if timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
		signals := captureCommandSignals(ctx)
		defer signals.stop()
		ctx = signals.Context
		client, url, token, err := lifecycleConnection()
		if err != nil {
			return err
		}
		defer client.CloseIdleConnections()
		client.Timeout = 0
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/events/agent?ids="+strings.Join(args, ",")+"&progress="+strconv.FormatBool(progress)+"&interval_ms="+strconv.FormatInt(interval.Milliseconds(), 10), nil)
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bearer "+token)
		response, err := client.Do(request)
		if err != nil {
			if ctx.Err() != nil {
				return context.Cause(ctx)
			}
			return clioutput.NewError(clioutput.Unavailable, "Could not open the event stream.", nil)
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != http.StatusOK {
			return remoteStatusError(response.StatusCode, clioutput.NewError(clioutput.Unavailable, "The server cannot provide filtered events; upgrade or inspect capabilities.", nil))
		}
		scanner := bufio.NewScanner(response.Body)
		scanner.Buffer(make([]byte, 4096), 2<<20)
		for {
			var event agentEvent
			if !scanner.Scan() {
				if ctx.Err() != nil {
					return context.Cause(ctx)
				}
				return clioutput.NewError(clioutput.Unavailable, "Event stream ended; reconnect for a fresh snapshot.", nil)
			}
			if json.Unmarshal(scanner.Bytes(), &event) != nil {
				return clioutput.NewError(clioutput.Unavailable, "Invalid or oversized event frame.", nil)
			}
			if event.SchemaVersion != 1 {
				return clioutput.NewError(clioutput.Conflict, "Unsupported event schema.", nil)
			}
			if err := clioutput.WriteNDJSON(cmd.OutOrStdout(), event); err != nil {
				return err
			}
			if event.Type == "resync_required" {
				return &streamEndedError{}
			}
		}
	}}
	command.Flags().Bool("progress", false, "Include bounded scalar progress samples (no chunk maps)")
	command.Flags().Duration("interval", time.Second, "Minimum per-job progress interval (250ms-1m)")
	command.Flags().Duration("timeout", 0, "Stop observing after this duration; jobs continue")
	configureContractValidation(command)
	return command
}

// A resync marker is already a complete NDJSON record; don't append a second error.
type streamEndedError struct{}

func (*streamEndedError) Error() string {
	return "Event resynchronization required; reconnect for a fresh snapshot."
}
func (*streamEndedError) Unwrap() error {
	return clioutput.NewError(clioutput.Unavailable, "Event resynchronization required.", nil)
}
func isStreamEnded(err error) bool { var ended *streamEndedError; return errors.As(err, &ended) }
func init()                        { rootCmd.AddCommand(newEventsCommand()) }

// Keep every selected snapshot within the client's 2 MiB frame bound, including
// worst-case JSON escaping. Agent streams never need arbitrary metadata sizes.
func agentDownloadItems(statuses []types.DownloadStatus) []downloadItem {
	items := downloadItems(statuses)
	for index := range items {
		item := &items[index]
		item.Filename = strings.Clone(item.Filename[:min(len(item.Filename), 1024)])
		item.Status = strings.Clone(item.Status[:min(len(item.Status), 64)])
		item.Phase = strings.Clone(item.Phase[:min(len(item.Phase), 64)])
	}
	return items
}
