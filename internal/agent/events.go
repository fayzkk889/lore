package agent

import "github.com/fayzkk889/lore/internal/engine"

// Event is the agent → UI stream. The TUI and the headless runner both
// consume these; the agent never prints anything itself.
type Event struct {
	Kind string
	// "text"         streamed assistant prose (Text)
	// "tool_start"   a tool call began (Tool, ToolID, Detail = input summary)
	// "tool_output"  a streamed line of shell/tool output (Tool, Text)
	// "tool_done"    a tool call finished (Tool, ToolID, OK, Detail = result summary)
	// "file_diff"    a file was (or is about to be) written (Path, OldContent, NewContent)
	// "verify"       verification stage report (OK, Detail = summary)
	// "retry"        transient engine failure, retrying (Attempt)
	// "usage"        cumulative token usage update (Usage)
	// "turn"         one model turn completed (Detail = stop reason)
	// "done"         the whole request finished (OK, Text = final assistant prose)
	// "error"        fatal error, agent stopped (Err)
	// "info"         harness notice worth showing (Text)

	Text       string
	Tool       string
	ToolID     string
	Detail     string
	Path       string
	OldContent string
	NewContent string
	OK         bool
	Attempt    int
	Usage      engine.Usage
	Err        error
}

type ApprovalDecision int

const (
	ApproveDeny ApprovalDecision = iota
	ApproveOnce
	ApproveAlways
)

type PermissionMode string

const (
	PermissionFullAuto PermissionMode = "full-auto"
	PermissionAutoSafe PermissionMode = "auto-safe"
	PermissionAsk      PermissionMode = "ask"
	PermissionReadOnly PermissionMode = "read-only"
)

// Approver lets a UI gate file writes. Approve is called from the agent
// goroutine and may block while the user decides.
type Approver interface {
	ApproveWrite(path, oldContent, newContent string) ApprovalDecision
}

// CommandApprover is an optional extension for UIs that also want to gate
// shell commands. Agents still accept older Approver implementations.
type CommandApprover interface {
	ApproveCommand(command string) ApprovalDecision
}

// ActionApprover is an optional extension for non-file, non-shell actions
// such as dependency setup, verification, and deletes.
type ActionApprover interface {
	ApproveAction(action, detail string) ApprovalDecision
}
