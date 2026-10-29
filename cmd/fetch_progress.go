package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SurgeDM/Surge/internal/transferui"
	"github.com/SurgeDM/Surge/internal/types"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

type progressSnapshot struct {
	ID, Filename, State   string
	Downloaded, Total     int64
	Speed                 float64
	Connections, Capacity int
}
type fetchProgress struct {
	mu          sync.Mutex
	latest      map[string]progressSnapshot
	writer      io.Writer
	mode, color string
	tty, ascii  bool
	fd          uintptr
	cancel      context.CancelFunc
	done        <-chan struct{}
}

func configureFetchProgress(command *cobra.Command) {
	command.Flags().String("progress", "auto", "Progress on stderr: auto, bar, plain, none")
	command.Flags().String("color", "auto", "Progress color: auto, always, never")
	command.Flags().Bool("ascii", false, "Use only ASCII progress decorations")
	command.Flags().BoolP("quiet", "q", false, "Suppress human progress and successful chatter")
}
func validateFetchProgress(cmd *cobra.Command) error {
	mode, _ := cmd.Flags().GetString("progress")
	color, _ := cmd.Flags().GetString("color")
	if mode != "auto" && mode != "bar" && mode != "plain" && mode != "none" {
		return usageError("--progress must be auto, bar, plain, or none.")
	}
	if color != "auto" && color != "always" && color != "never" {
		return usageError("--color must be auto, always, or never.")
	}
	return nil
}
func startFetchProgress(cmd *cobra.Command) *fetchProgress {
	mode, _ := cmd.Flags().GetString("progress")
	quiet, _ := cmd.Flags().GetBool("quiet")
	if quiet || mode == "none" {
		return nil
	}
	writer := cmd.ErrOrStderr()
	fd := uintptr(0)
	tty := false
	if file, ok := writer.(*os.File); ok {
		fd = file.Fd()
		tty = term.IsTerminal(fd) && os.Getenv("TERM") != "dumb"
	}
	jsonOutput, _ := cmd.Flags().GetBool("json")
	version, _ := cmd.Flags().GetString("output-version")
	if mode == "auto" {
		if tty {
			mode = "bar"
		} else if jsonOutput || version == "1" {
			return nil
		} else {
			mode = "plain"
		}
	}
	if mode == "bar" && !tty {
		mode = "plain"
	}
	color, _ := cmd.Flags().GetString("color")
	ascii, _ := cmd.Flags().GetBool("ascii")
	if color == "auto" {
		if tty && os.Getenv("NO_COLOR") == "" {
			color = "always"
		} else {
			color = "never"
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	progress := &fetchProgress{latest: make(map[string]progressSnapshot), writer: writer, mode: mode, color: color, tty: tty, ascii: ascii, fd: fd, cancel: cancel, done: done}
	go func() { defer close(done); progress.render(ctx) }()
	return progress
}

// Capture only small scalar metadata. Never format or write while holding mu.
func (p *fetchProgress) update(status *types.DownloadStatus) {
	if p == nil || status == nil {
		return
	}
	state := status.Status
	if status.Phase != "" {
		state = status.Phase
	}
	rate := status.Speed
	if state != "downloading" || !transferui.FiniteRate(rate) {
		rate = 0
	}
	snapshot := progressSnapshot{ID: status.ID, Filename: strings.Clone(status.Filename[:min(len(status.Filename), 1024)]), State: state, Downloaded: status.Downloaded, Total: status.TotalSize, Speed: rate, Connections: status.Connections, Capacity: status.MaxConnections}
	p.mu.Lock()
	p.latest[status.ID] = snapshot
	p.mu.Unlock()
}
func (p *fetchProgress) stop() {
	if p == nil {
		return
	}
	p.cancel()
	// A stopped terminal owns only this rendering goroutine. Never wait on it
	// while shutting down transfers or writing the final stdout receipt.
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-p.done:
	case <-timer.C:
	}
}
func (p *fetchProgress) snapshots() []progressSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := make([]progressSnapshot, 0, len(p.latest))
	for _, snapshot := range p.latest {
		result = append(result, snapshot)
	}
	return result
}
func (p *fetchProgress) render(ctx context.Context) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	states := make(map[string]string)
	lastPlain := make(map[string]time.Time)
	previous := ""
	rows := 0
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			snapshots := p.snapshots()
			if len(snapshots) == 0 {
				continue
			}
			sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].ID < snapshots[j].ID })
			if p.mode == "plain" {
				for _, snapshot := range snapshots {
					if states[snapshot.ID] == snapshot.State && now.Sub(lastPlain[snapshot.ID]) < 5*time.Second {
						continue
					}
					states[snapshot.ID] = snapshot.State
					lastPlain[snapshot.ID] = now
					if _, err := fmt.Fprintln(p.writer, p.progressLine(snapshot, 0)); err != nil {
						return
					}
				}
				continue
			}
			width, height, err := term.GetSize(p.fd)
			if err != nil || width < 1 || height < 1 {
				continue
			}
			width = min(width, 500)
			height = min(height, 20)
			lines := p.progressFrame(snapshots, width, height)
			frame := strings.Join(lines, "\n")
			if frame == previous {
				continue
			}
			previous = frame
			var output strings.Builder
			if rows > 0 {
				fmt.Fprintf(&output, "\x1b[%dA", rows)
			}
			for _, line := range lines {
				output.WriteString("\r\x1b[2K")
				output.WriteString(line)
				output.WriteByte('\n')
			}
			for i := len(lines); i < rows; i++ {
				output.WriteString("\r\x1b[2K\n")
			}
			if rows > len(lines) {
				fmt.Fprintf(&output, "\x1b[%dA", rows-len(lines))
			}
			rows = len(lines)
			if _, err := io.WriteString(p.writer, output.String()); err != nil {
				return
			}
		}
	}
}
func (p *fetchProgress) progressLine(s progressSnapshot, width int) string {
	name := transferui.SafeText(s.Filename)
	if name == "" {
		name = s.ID
	}
	state := transferui.SafeText(s.State)
	line := fmt.Sprintf("%s  %s  %s", name, state, transferui.Bytes(s.Downloaded))
	if s.Total > 0 {
		percent := min(100, float64(max(0, s.Downloaded))*100/float64(s.Total))
		line = fmt.Sprintf("%s  %s  %.1f%%  %s / %s", name, state, percent, transferui.Bytes(s.Downloaded), transferui.Bytes(s.Total))
	}
	if state == "downloading" {
		line += fmt.Sprintf("  %s/s", transferui.ByteValue(s.Speed))
		if eta, known := transferui.ETA(s.Total, s.Downloaded, s.Speed); known {
			line += "  ETA ~" + transferui.Duration(eta)
		} else {
			line += "  ETA --"
		}
		if s.Capacity > 0 {
			line += fmt.Sprintf("  connections %d/%d", s.Connections, s.Capacity)
		} else {
			line += fmt.Sprintf("  connections %d", s.Connections)
		}
	}
	if width > 0 {
		marker := "..."
		if !p.ascii {
			marker = "\u2026"
		}
		line = ansi.Truncate(line, max(0, width-1), marker)
	}
	if p.color == "always" {
		line = "\x1b[36m" + line + "\x1b[0m"
	}
	return line
}
func (p *fetchProgress) progressFrame(snapshots []progressSnapshot, width, height int) []string {
	active, queued, done, failed, unknown := 0, 0, 0, 0, 0
	var downloaded float64
	rate := 0.0
	for _, s := range snapshots {
		downloaded += float64(max(0, s.Downloaded))
		rate += s.Speed
		if s.Total <= 0 {
			unknown++
		}
		switch s.State {
		case "completed":
			done++
		case "error":
			failed++
		case "queued", "retrying":
			queued++
		default:
			active++
		}
	}
	summary := fmt.Sprintf("%d active / %d queued / %d done / %d failed  %s  %s/s", active, queued, done, failed, transferui.ByteValue(downloaded), transferui.ByteValue(rate))
	if unknown > 0 {
		summary += fmt.Sprintf("  %d unknown-size", unknown)
	}
	lines := []string{ansi.Truncate(summary, max(0, width-1), "...")}
	visible := min(len(snapshots), max(0, (height-2)/2), 6)
	for _, snapshot := range snapshots[:visible] {
		name := transferui.SafeText(snapshot.Filename)
		if name == "" {
			name = snapshot.ID
		}
		name = truncateProgressFilename(name, max(4, width/2), p.ascii)
		state := transferui.SafeText(snapshot.State)
		head := name + "  " + state
		if snapshot.Total > 0 {
			fraction := min(1, float64(max(0, snapshot.Downloaded))/float64(snapshot.Total))
			head += fmt.Sprintf("  %.1f%%", fraction*100)
			if width >= 70 {
				fill := int(fraction * 10)
				head += "  [" + strings.Repeat("=", fill) + strings.Repeat("-", 10-fill) + "]"
			}
		}
		lines = append(lines, ansi.Truncate(head, max(0, width-1), "..."))
		metrics := snapshot
		metrics.Filename = ""
		metrics.ID = ""
		metrics.State = snapshot.State
		lines = append(lines, "  "+ansi.Truncate(strings.TrimSpace(p.progressLine(metrics, 0)), max(0, width-3), "..."))
	}
	if len(snapshots) > visible && height > 2 {
		lines = append(lines, ansi.Truncate(fmt.Sprintf("%d more downloads", len(snapshots)-visible), max(0, width-1), "..."))
	}
	return lines
}

func truncateProgressFilename(name string, width int, ascii bool) string {
	marker := "\u2026"
	if ascii {
		marker = "..."
	}
	if ansi.StringWidth(name) <= width {
		return name
	}
	dot := strings.LastIndexByte(name, '.')
	if dot > 0 && ansi.StringWidth(name[dot:]) < width/2 {
		return ansi.Truncate(name[:dot], width-ansi.StringWidth(name[dot:]), marker) + name[dot:]
	}
	return ansi.Truncate(name, width, marker)
}
