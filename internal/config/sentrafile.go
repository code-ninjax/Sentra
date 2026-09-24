// Package config parses Sentrafiles — Sentra's build configuration format.
//
// Sentrafile is intentionally NOT Dockerfile-compatible. It is a small,
// line-oriented format with no shell and no per-instruction flags. Every
// directive is a bare keyword followed by simple arguments.
//
// # Syntax (locked for MVP)
//
//	# comment                    — full-line only; '#' mid-line is literal
//	base <image>                 — required, must be the first directive
//	workdir <abs-path>           — absolute paths only
//	copy <src> <dest>            — paths are always relative to build context
//	exec <cmd> [args...]         — argv form, no implicit shell
//	env KEY=VALUE [KEY=VALUE...] — one or more pairs per line
//	expose <port>[/proto] ...    — ports are metadata, not host publishing
//	start <cmd> [args...]        — container entrypoint, at most one, last
//	cache key=<value>            — optional manual cache-key override
//	secure rootless=<bool> readonly=<bool> seccomp=<default|strict>
//
// Blank lines are ignored. Directives are case-insensitive (BASE == base).
// Arguments are tokenized with shell-like quoting: single quotes are
// literal, double quotes honor backslash escapes, and bare backslash
// escapes the next character. There is no variable expansion, globbing, or
// command substitution — those belong to exec's argv, not the parser.
//
// A Sentrafile is a *build plan*, not a script. The parser turns it into a
// flat, ordered list of Steps. Nothing downstream re-reads the raw text, so
// changing the syntax later only touches this package.
package config

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Directive is the lowercase keyword that identifies a Sentrafile line.
type Directive string

const (
	DirectiveBase    Directive = "base"
	DirectiveWorkdir Directive = "workdir"
	DirectiveCopy    Directive = "copy"
	DirectiveExec    Directive = "exec"
	DirectiveEnv     Directive = "env"
	DirectiveExpose  Directive = "expose"
	DirectiveStart   Directive = "start"
	DirectiveCache   Directive = "cache"
	DirectiveSecure  Directive = "secure"
)

// Step is one executable unit of a build plan. The executor dispatches on
// Type and reads only the fields that Type populates.
type Step struct {
	Type Directive `json:"type"`
	Line int       `json:"line"`

	// base
	Image string `json:"image,omitempty"`

	// workdir
	Path string `json:"path,omitempty"`

	// copy
	Src  string `json:"src,omitempty"`
	Dest string `json:"dest,omitempty"`

	// exec, start
	Argv []string `json:"argv,omitempty"`

	// env
	Env []string `json:"env,omitempty"`

	// expose
	Ports []string `json:"ports,omitempty"`

	// Cache is a manual cache-key override attached by a preceding
	// "cache key=<value>" directive. Empty means "use the automatic
	// file-hash key".
	Cache string `json:"cache,omitempty"`
}

// Summary renders a step's arguments for `sentra config` output and error
// messages.
func (s Step) Summary() string {
	switch s.Type {
	case DirectiveBase:
		return s.Image
	case DirectiveWorkdir:
		return s.Path
	case DirectiveCopy:
		return s.Src + " -> " + s.Dest
	case DirectiveExec, DirectiveStart:
		return strings.Join(s.Argv, " ")
	case DirectiveEnv:
		return strings.Join(s.Env, " ")
	case DirectiveExpose:
		return strings.Join(s.Ports, " ")
	}
	return ""
}

// Security is the build's declared posture, set by the secure directive.
// It is metadata here; workstream 5 (security defaults) is what enforces it.
type Security struct {
	Rootless bool   `json:"rootless"`
	Readonly bool   `json:"readonly"`
	Seccomp  string `json:"seccomp"` // "default" | "strict"
}

// DefaultSecurity is the posture used when no secure directive is present.
// It matches the runtime defaults established in workstream 1: rootless on,
// rootfs read-only, permissive seccomp until workstream 5 ships a profile.
func DefaultSecurity() Security {
	return Security{Rootless: true, Readonly: true, Seccomp: "default"}
}

// Plan is a parsed Sentrafile: the base image, the ordered build steps,
// and the security posture.
type Plan struct {
	Base     string   `json:"base"`
	Steps    []Step   `json:"steps"`
	Security Security `json:"security"`
}

// Error is a parse failure tied to a source line so the user can fix the
// exact spot instead of guessing.
type Error struct {
	Line int
	Msg  string
}

func (e *Error) Error() string {
	return fmt.Sprintf("Sentrafile:%d: %s", e.Line, e.Msg)
}

func errf(line int, format string, args ...any) error {
	return &Error{Line: line, Msg: fmt.Sprintf(format, args...)}
}

// ParseFile reads and parses the Sentrafile at path.
func ParseFile(path string) (*Plan, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	plan, err := Parse(data)
	if err != nil {
		return nil, err
	}
	return plan, nil
}

// Parse turns raw Sentrafile bytes into a Plan.
func Parse(data []byte) (*Plan, error) {
	plan := &Plan{Security: DefaultSecurity()}
	seenBase := false
	seenSecure := false
	seenStart := false

	// pendingCache holds a "cache key=" override until the next step
	// consumes it. A cache directive with no step below it is an error.
	pendingCache := ""
	addStep := func(s Step) {
		s.Cache = pendingCache
		pendingCache = ""
		plan.Steps = append(plan.Steps, s)
	}

	for i, raw := range splitLines(data) {
		line := i + 1
		fields, err := tokenize(raw, line)
		if err != nil {
			return nil, err
		}
		if len(fields) == 0 {
			continue // blank or comment-only line
		}

		directive := Directive(strings.ToLower(fields[0]))
		args := fields[1:]

		switch directive {
		case DirectiveBase:
			if seenBase {
				return nil, errf(line, "duplicate base directive")
			}
			if i != 0 {
				return nil, errf(line, "base must be the first directive")
			}
			if len(args) != 1 || args[0] == "" {
				return nil, errf(line, "base requires exactly one image reference")
			}
			plan.Base = args[0]
			seenBase = true

		case DirectiveWorkdir:
			if len(args) != 1 || args[0] == "" {
				return nil, errf(line, "workdir requires exactly one path")
			}
			if !strings.HasPrefix(args[0], "/") {
				return nil, errf(line, "workdir must be absolute, got %q", args[0])
			}
			addStep(Step{Type: DirectiveWorkdir, Line: line, Path: args[0]})

		case DirectiveCopy:
			if len(args) != 2 {
				return nil, errf(line, "copy requires <src> <dest>")
			}
			addStep(Step{
				Type: DirectiveCopy, Line: line, Src: args[0], Dest: args[1],
			})

		case DirectiveExec:
			if len(args) == 0 {
				return nil, errf(line, "exec requires a command")
			}
			addStep(Step{Type: DirectiveExec, Line: line, Argv: args})

		case DirectiveEnv:
			if len(args) == 0 {
				return nil, errf(line, "env requires at least one KEY=VALUE pair")
			}
			for _, pair := range args {
				key, _, ok := strings.Cut(pair, "=")
				if !ok || key == "" {
					return nil, errf(line, "env expects KEY=VALUE, got %q", pair)
				}
			}
			addStep(Step{Type: DirectiveEnv, Line: line, Env: args})

		case DirectiveExpose:
			if len(args) == 0 {
				return nil, errf(line, "expose requires at least one port")
			}
			for _, port := range args {
				if !validPort(port) {
					return nil, errf(line, "invalid port %q, want <number>[/tcp|udp|sctp]", port)
				}
			}
			addStep(Step{Type: DirectiveExpose, Line: line, Ports: args})

		case DirectiveStart:
			if seenStart {
				return nil, errf(line, "duplicate start directive")
			}
			if len(args) == 0 {
				return nil, errf(line, "start requires a command")
			}
			addStep(Step{Type: DirectiveStart, Line: line, Argv: args})
			seenStart = true

		case DirectiveCache:
			if len(args) != 1 {
				return nil, errf(line, "cache requires key=<value>")
			}
			key, value, ok := strings.Cut(args[0], "=")
			if !ok || key != "key" || value == "" {
				return nil, errf(line, "cache expects key=<value>, got %q", args[0])
			}
			if pendingCache != "" {
				return nil, errf(line, "cache directive must be followed by a step before another cache")
			}
			pendingCache = value

		case DirectiveSecure:
			if seenSecure {
				return nil, errf(line, "duplicate secure directive")
			}
			sec, err := parseSecure(args, line)
			if err != nil {
				return nil, err
			}
			plan.Security = sec
			seenSecure = true

		default:
			return nil, errf(line, "unknown directive %q", fields[0])
		}
	}

	if !seenBase {
		return nil, &Error{Line: 1, Msg: "missing base directive (Sentrafile must start with 'base <image>')"}
	}
	if pendingCache != "" {
		return nil, &Error{
			Line: 1,
			Msg:  "cache key= must be followed by the step it applies to (a trailing cache directive has nothing to attach to)",
		}
	}
	return plan, nil
}

func parseSecure(args []string, line int) (Security, error) {
	sec := DefaultSecurity()
	if len(args) == 0 {
		return sec, errf(line, "secure requires at least one of rootless=, readonly=, seccomp=")
	}
	for _, arg := range args {
		key, value, ok := strings.Cut(arg, "=")
		if !ok {
			return sec, errf(line, "secure expects key=value, got %q", arg)
		}
		switch key {
		case "rootless":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return sec, errf(line, "secure rootless= expects a bool, got %q", value)
			}
			sec.Rootless = b
		case "readonly":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return sec, errf(line, "secure readonly= expects a bool, got %q", value)
			}
			sec.Readonly = b
		case "seccomp":
			if value != "default" && value != "strict" {
				return sec, errf(line, "secure seccomp= expects default|strict, got %q", value)
			}
			sec.Seccomp = value
		default:
			return sec, errf(line, "unknown secure option %q", key)
		}
	}
	return sec, nil
}

func validPort(port string) bool {
	num, proto, hasProto := strings.Cut(port, "/")
	n, err := strconv.Atoi(num)
	if err != nil || n < 1 || n > 65535 {
		return false
	}
	if !hasProto {
		return true
	}
	switch proto {
	case "tcp", "udp", "sctp":
		return true
	}
	return false
}

// splitLines normalizes line endings and strips full-line comments and
// trailing whitespace, returning the significant lines in order.
func splitLines(data []byte) []string {
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// tokenize splits a directive line into arguments using shell-like
// quoting rules: single quotes are literal, double quotes honor backslash
// escapes, and a bare backslash escapes the next character. There is no
// variable expansion, globbing, or command substitution — Sentrafile is a
// plan, not a shell.
func tokenize(line string, lineNo int) ([]string, error) {
	var (
		args []string
		cur  strings.Builder
		open bool
	)
	flush := func() {
		if open {
			args = append(args, cur.String())
			cur.Reset()
			open = false
		}
	}

	for i := 0; i < len(line); i++ {
		switch c := line[i]; c {
		case ' ', '\t':
			flush()

		case '\'':
			end := strings.IndexByte(line[i+1:], '\'')
			if end < 0 {
				return nil, errf(lineNo, "unterminated single quote")
			}
			cur.WriteString(line[i+1 : i+1+end])
			open = true
			i += end + 1

		case '"':
			closed := false
			open = true
			for i++; i < len(line); i++ {
				if line[i] == '\\' && i+1 < len(line) && isEscapable(line[i+1]) {
					cur.WriteByte(line[i+1])
					i++
					continue
				}
				if line[i] == '"' {
					closed = true
					break
				}
				cur.WriteByte(line[i])
			}
			if !closed {
				return nil, errf(lineNo, "unterminated double quote")
			}

		case '\\':
			if i+1 < len(line) {
				i++
				cur.WriteByte(line[i])
				open = true
			}

		default:
			cur.WriteByte(c)
			open = true
		}
	}
	flush()
	return args, nil
}

func isEscapable(c byte) bool {
	return c == '"' || c == '\\' || c == '$' || c == '`'
}

// ReadAndParse is a convenience for callers holding a reader.
func ReadAndParse(r io.Reader) (*Plan, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return Parse(data)
}
