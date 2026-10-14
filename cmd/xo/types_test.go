package main

import (
	"encoding/json"

	"github.com/xo-lang/xo/internal/gobind"
)

// The JSON shapes the tests read from xo's output. They were the Go
// command line's own types before decision 0215 deleted it; the self
// hosted xo prints the same JSON (selfhost/cli).

// diagnostic is one diagnostic of `--json` output (spec/core.md 10).
type diagnostic struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Span     struct {
		File    string `json:"file"`
		Line    int    `json:"line"`
		Col     int    `json:"col"`
		EndLine int    `json:"end_line"`
		EndCol  int    `json:"end_col"`
	} `json:"span"`
	Notes []json.RawMessage `json:"notes"`
	Fixes []json.RawMessage `json:"fixes"`
}

// bindResult is one package of `xo bind --json` (the helper xo-gobind's
// result, internal/gobind).
type bindResult = gobind.CLIResult

// cBindResult is one header of `xo bind --c --json`.
type cBindResult struct {
	Header  string            `json:"header"`
	Path    string            `json:"path,omitempty"`
	Decl    string            `json:"decl,omitempty"`
	Bound   int               `json:"bound"`
	Total   int               `json:"total"`
	Skipped map[string]string `json:"skipped,omitempty"`
	Error   string            `json:"error,omitempty"`
}

// auditReport is the result of `xo audit --json` (decision 0156).
type auditReport struct {
	Module        string             `json:"module"`
	Dir           string             `json:"dir"`
	Dependencies  []auditDep         `json:"dependencies"`
	Attenuations  []auditAttenuation `json:"attenuations"`
	Capabilities  []auditCapability  `json:"capabilities_passed"`
	UnusedVouches []auditVouch       `json:"unused_vouches"`
	Summary       auditSummary       `json:"summary"`
	Diagnostics   []diagnostic       `json:"diagnostics"`
}

type auditDep struct {
	Path      string        `json:"path"`
	Version   string        `json:"version,omitempty"`
	Direct    bool          `json:"direct"`
	Workspace bool          `json:"workspace,omitempty"`
	Effects   []string      `json:"effects"`
	Locked    *[]string     `json:"locked_effects,omitempty"`
	Lock      string        `json:"lock"`
	Added     []string      `json:"added_effects,omitempty"`
	Ceiling   string        `json:"ceiling,omitempty"`
	Escapes   []auditEscape `json:"escape_hatches"`
	LogTag    string        `json:"log_attribution,omitempty"`
	LogSites  []auditLoc    `json:"log_sites,omitempty"`
}

type auditLoc struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Call string `json:"call"`
}

type auditEscape struct {
	Kind    string    `json:"kind"`
	Target  string    `json:"target"`
	File    string    `json:"file"`
	Line    int       `json:"line"`
	Claim   *[]string `json:"claimed_effects,omitempty"`
	Vouched bool      `json:"vouched"`
	Vouch   string    `json:"vouch,omitempty"`
	Effects []string  `json:"effects"`
}

type auditAttenuation struct {
	File       string   `json:"file"`
	Line       int      `json:"line"`
	Capability string   `json:"capability"`
	Method     string   `json:"method"`
	Args       []string `json:"args,omitempty"`
	Constant   bool     `json:"constant"`
}

type auditCapability struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	Callee     string `json:"callee"`
	Dependency string `json:"dependency"`
	Capability string `json:"capability"`
	Attenuated bool   `json:"attenuated"`
	Via        string `json:"via,omitempty"`
}

type auditVouch struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

type auditSummary struct {
	Dependencies     int `json:"dependencies"`
	Unvouched        int `json:"unvouched_escape_hatches"`
	Widened          int `json:"widened"`
	Unlocked         int `json:"unlocked"`
	UnattenuatedCaps int `json:"unattenuated_capabilities_passed"`
	Errors           int `json:"errors"`
}
