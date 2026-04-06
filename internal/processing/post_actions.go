package processing

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/SurgeDM/Surge/internal/config"
	"github.com/SurgeDM/Surge/internal/utils"
)

// PostActionContext holds information about a completed download for template substitution.
type PostActionContext struct {
	Filename string
	FilePath string
	Size     int64
	Speed    float64
	Duration time.Duration
	ID       string
	Error    string
}

// shellEscape quotes a string so it is safe to embed in a shell command.
// On Unix it wraps the value in single quotes, escaping any internal single
// quotes with the '\'' idiom.  On Windows it wraps in double quotes and
// escapes internal double quotes with "".
func shellEscape(s string) string {
	if runtime.GOOS == "windows" {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// expandTemplate replaces {variable} placeholders with shell-escaped values.
func expandTemplate(template string, ctx PostActionContext) string {
	r := strings.NewReplacer(
		"{filename}", shellEscape(ctx.Filename),
		"{filepath}", shellEscape(ctx.FilePath),
		"{size}", fmt.Sprintf("%d", ctx.Size),
		"{speed}", fmt.Sprintf("%.2f", ctx.Speed),
		"{duration}", ctx.Duration.Truncate(time.Second).String(),
		"{id}", shellEscape(ctx.ID),
		"{error}", shellEscape(ctx.Error),
	)
	return r.Replace(template)
}

// RunPostActions runs configured post-download actions.
// Errors are logged but never propagated to prevent post-action failures from
// corrupting the download lifecycle.
func RunPostActions(settings config.PostDownloadActions, ctx PostActionContext, isError bool) {
	var cmd string
	if isError {
		cmd = settings.OnErrorCommand
	} else {
		cmd = settings.OnCompleteCommand
	}
	if cmd == "" {
		return
	}

	expanded := expandTemplate(cmd, ctx)
	utils.Debug("PostAction: executing %q", expanded)

	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		c = exec.Command("cmd", "/C", expanded)
	} else {
		c = exec.Command("sh", "-c", expanded)
	}

	if output, err := c.CombinedOutput(); err != nil {
		utils.Debug("PostAction: command failed: %v (output: %s)", err, string(output))
	} else {
		utils.Debug("PostAction: command succeeded (output: %s)", string(output))
	}
}
