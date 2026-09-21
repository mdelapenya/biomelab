package regent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mdelapenya/biomelab/internal/command"
)

// Step is one re_gent step — a turn of agent activity, identified by a
// content-addressed hash. Mirrors the JSON shape emitted by
// `rgt log --json`, with the bits biomelab cares about pulled out for
// easy rendering. Unknown fields are ignored.
type Step struct {
	Hash      string
	Timestamp time.Time
	Origin    string // e.g. "claude_code"

	// HumanPrompt is the user prompt that triggered this step (extracted
	// from messages[].message.content for type=user). Empty when the
	// step has no associated user message.
	HumanPrompt string

	// AgentReply is the assistant's text reply for this step (extracted
	// from messages[].message.content for type=assistant, concatenating
	// any text-type content blocks). Empty when the step has no reply
	// or only tool_use blocks.
	AgentReply string

	// Tools is the ordered list of tool invocations that ran in this
	// step. Comes from the top-level `causes` array.
	Tools []ToolCall
}

// ToolCall captures a single tool invocation. Args is the raw JSON map
// so renderers can surface arbitrary tool-specific fields (file_path,
// command, query, etc.) without us having to model every tool.
type ToolCall struct {
	Name string
	Args map[string]any
}

// LogJSON runs `rgt log --json --limit <limit>` in wtPath and parses the
// result. Returns the session ID and a slice of Steps (most recent first,
// matching rgt's own ordering). Empty slice + nil error when there's
// no .regent/ or no activity yet.
func LogJSON(wtPath string, limit int) (sessionID string, steps []Step, err error) {
	data, err := LogJSONRaw(wtPath, limit)
	if err != nil || data == nil {
		return "", nil, err
	}
	return parseLogJSON(data)
}

// LogJSONRaw runs `rgt log --json` and returns the raw bytes so callers
// can write them to disk (for "Export to JSON" flows) without re-shelling.
// Returns nil + nil when wtPath is missing or has no .regent/ — the
// caller treats that as "nothing to export". Errors from rgt itself
// propagate as non-nil err.
func LogJSONRaw(wtPath string, limit int) ([]byte, error) {
	if wtPath == "" {
		return nil, nil
	}
	if _, err := os.Stat(filepath.Join(wtPath, ".regent")); err != nil {
		return nil, nil
	}
	bin, err := exec.LookPath("rgt")
	if err != nil {
		return nil, fmt.Errorf("rgt not found in PATH")
	}
	args := []string{"log", "--json"}
	if limit > 0 {
		args = append(args, "--limit", strconv.Itoa(limit))
	}
	cmd := command.Background(bin, args...)
	cmd.Dir = wtPath
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("rgt log: %w", err)
	}
	return out, nil
}

// rawStep mirrors the on-the-wire JSON. Decoupled from Step so the public
// type stays clean and a schema bump only touches this file.
type rawStep struct {
	Hash      string       `json:"hash"`
	Timestamp time.Time    `json:"timestamp"`
	Origin    string       `json:"origin"`
	Causes    []rawCause   `json:"causes"`
	Messages  []rawMessage `json:"messages"`
}

type rawCause struct {
	Tool string         `json:"tool"`
	Args map[string]any `json:"args"`
}

type rawMessage struct {
	Type    string          `json:"type"` // "user" or "assistant"
	Message json.RawMessage `json:"message"`
}

// parseLogJSON converts rgt's JSON output to Step values. Extracted from
// LogJSON so tests can feed fixture bytes directly without shelling out.
func parseLogJSON(data []byte) (sessionID string, steps []Step, err error) {
	var top struct {
		SessionID string    `json:"session_id"`
		Steps     []rawStep `json:"steps"`
	}
	if err := json.Unmarshal(data, &top); err != nil {
		return "", nil, fmt.Errorf("parse rgt log json: %w", err)
	}

	steps = make([]Step, 0, len(top.Steps))
	for _, rs := range top.Steps {
		s := Step{
			Hash:      rs.Hash,
			Timestamp: rs.Timestamp,
			Origin:    rs.Origin,
		}
		for _, c := range rs.Causes {
			s.Tools = append(s.Tools, ToolCall{Name: c.Tool, Args: c.Args})
		}
		for _, m := range rs.Messages {
			switch m.Type {
			case "user":
				if t := extractUserText(m.Message); t != "" {
					if s.HumanPrompt != "" {
						s.HumanPrompt += "\n\n"
					}
					s.HumanPrompt += t
				}
			case "assistant":
				if t := extractAssistantText(m.Message); t != "" {
					if s.AgentReply != "" {
						s.AgentReply += "\n\n"
					}
					s.AgentReply += t
				}
			}
		}
		steps = append(steps, s)
	}
	return top.SessionID, steps, nil
}

// extractUserText pulls the prompt text out of a user-message blob. Claude
// Code stores user messages as `{"role": "user", "content": "…"}` with
// content as a plain string.
func extractUserText(raw json.RawMessage) string {
	var m struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	// content can be a plain string OR a list of content blocks.
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		return s
	}
	return joinTextBlocks(m.Content)
}

// extractAssistantText pulls the visible reply text out of an assistant
// message blob. Assistant messages from Claude are an array of content
// blocks of type "text" or "tool_use"; we keep the text ones and drop
// the rest (the tools surface separately in the Tools slice).
func extractAssistantText(raw json.RawMessage) string {
	var m struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		return s
	}
	return joinTextBlocks(m.Content)
}

// joinTextBlocks walks a content-block array and concatenates the text
// of `type: "text"` entries. Tool-use entries are intentionally ignored
// because they're already represented in the Step.Tools slice.
func joinTextBlocks(raw json.RawMessage) string {
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	var out strings.Builder
	for _, b := range blocks {
		if b.Type != "text" {
			continue
		}
		t := strings.TrimSpace(b.Text)
		if t == "" {
			continue
		}
		if out.Len() > 0 {
			out.WriteString("\n\n")
		}
		out.WriteString(t)
	}
	return out.String()
}

// ShortHash returns the first 8 chars of a step's hash, matching rgt's
// own log abbreviation. Convenience for renderers.
func (s Step) ShortHash() string {
	if len(s.Hash) <= 8 {
		return s.Hash
	}
	return s.Hash[:8]
}
