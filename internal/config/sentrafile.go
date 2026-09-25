// Package config parses Sentrafiles — Sentra's build configuration format.
//
// Sentrafile is intentionally NOT Dockerfile-compatible. It is a small,
// line-oriented format with no shell and no per-instruction flags. Every
// directive is a bare keyword followed by simple arguments.
//
// # Syntax (locked for MVP)
//
//	# comment                    — full-line only; '#' mid-line is literal
//	base <image> [as <name>]     — required, first; each base starts a stage
//	workdir <abs-path>           — absolute paths only
//	copy [--from=<stage>] <src> <dest>
//	exec <cmd> [args...]         — argv form, no implicit shell
//	env KEY=VALUE [KEY=VALUE...] — one or more pairs per line
//	expose <port>[/proto] ...    — ports are metadata, not host publishing
//	start <cmd> [args...]        — container entrypoint, at most one
//	cache key=<value>            — optional manual cache-key override
//	secure rootless=<bool> readonly=<bool> seccomp=<default|strict>
//
// A Sentrafile may declare several stages. Each `base` line opens a new
// stage, optionally naming it with `as <name>`. A later stage can start
// from an earlier one by naming it in its own `base`, or can lift files out
// of it with `copy --from=<name>`. Only the final stage produces the image,
// so every earlier stage must be named. The last stage carries the
// environment, ports and entrypoint the image is run with.
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

	// From names the stage a copy reads from instead of the build context.
	// Empty means "read from the build context".
	From string `json:"from,omitempty"`

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

// Stage is one base image plus the steps applied to it. A single-stage
// Sentrafile parses into exactly one unnamed stage.
type Stage struct {
	Name  string `json:"name,omitempty"`
	Base  string `json:"base"`
	Steps []Step `json:"steps"`
}

// Plan is a parsed Sentrafile: one or more stages plus the security
// posture. The final stage is the image the build produces.
type Plan struct {
	Stages   []Stage  `json:"stages"`
	Security Security `json:"security"`
}

// Final returns the stage that becomes the image.
func (p *Plan) Final() Stage { return p.Stages[len(p.Stages)-1] }

// StageNames lists the named stages in declaration order.
func (p *Plan) StageNames() []string {
	var names []string
	for _, s := range p.Stages {
		if s.Name != "" {
			names = append(names, s.Name)
		}
	}
	return names
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
	seenSecure := false
	seenStart := false

	// pendingCache holds a "cache key=" override until the next step
	// consumes it. A cache directive with no step below it is an error.
	pendingCache := ""
	addStep := func(s Step) {
		s.Cache = pendingCache
		pendingCache = ""
		cur := &plan.Stages[len(plan.Stages)-1]
		cur.Steps = append(cur.Steps, s)
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

		// Every directive except base needs an open stage to belong to.
		if directive != DirectiveBase && len(plan.Stages) == 0 {
			return nil, errf(line, "%s must come after a base directive", directive)
		}

		switch directive {
		case DirectiveBase:
			name, base, err := parseBase(args, line)
			if err != nil {
				return nil, err
			}
			if len(plan.Stages) == 0 && i != 0 {
				return nil, errf(line, "base must be the first directive")
			}
			for _, existing := range plan.Stages {
				if name != "" && existing.Name == name {
					return nil, errf(line, "duplicate stage name %q", name)
				}
			}
			// A base is a stage reference only when it names a stage
			// declared above it. Everything else is an image reference —
			// "alpine" is an image, not a misspelled stage.
			plan.Stages = append(plan.Stages, Stage{Name: name, Base: base})

		case DirectiveWorkdir:
			if len(args) != 1 || args[0] == "" {
				return nil, errf(line, "workdir requires exactly one path")
			}
			if !strings.HasPrefix(args[0], "/") {
				return nil, errf(line, "workdir must be absolute, got %q", args[0])
			}
			addStep(Step{Type: DirectiveWorkdir, Line: line, Path: args[0]})

		case DirectiveCopy:
			from, hasFrom, positional := stripFlags(args)
			if hasFrom && from == "" {
				return nil, errf(line, "copy --from= needs a stage name")
			}
			if len(positional) != 2 {
				return nil, errf(line, "copy requires [--from=<stage>] <src> <dest>")
			}
			if from != "" && !stageExists(plan, from) {
				return nil, errf(line, "copy --from=%s references unknown stage", from)
			}
			addStep(Step{
				Type: DirectiveCopy, Line: line,
				Src: positional[0], Dest: positional[1], From: from,
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

	if len(plan.Stages) == 0 {
		return nil, &Error{Line: 1, Msg: "missing base directive (Sentrafile must start with 'base <image>')"}
	}
	// Every stage but the last must be named: an unnamed stage cannot be
	// referenced, so it would be built and thrown away.
	for i, s := range plan.Stages {
		if i < len(plan.Stages)-1 && s.Name == "" {
			return nil, &Error{
				Line: 1,
				Msg:  "every stage except the last must be named: base <image> as <name>",
			}
		}
	}
	if pendingCache != "" {
		return nil, &Error{
			Line: 1,
			Msg:  "cache key= must be followed by the step it applies to (a trailing cache directive has nothing to attach to)",
		}
	}
	return plan, nil
}

// parseBase reads `base <ref> [as <name>]`, returning the stage name (empty
// when unnamed) and the base reference.
func parseBase(args []string, line int) (name, base string, err error) {
	switch len(args) {
	case 1:
		return "", args[0], nil
	case 3:
		if !strings.EqualFold(args[1], "as") {
			return "", "", errf(line, "base expects <image> [as <name>], got %q", args[1])
		}
		if args[2] == "" {
			return "", "", errf(line, "stage name after 'as' is empty")
		}
		if !looksLikeStageName(args[2]) {
			return "", "", errf(line, "invalid stage name %q (use letters, digits, dashes, underscores)", args[2])
		}
		return args[2], args[0], nil
	default:
		return "", "", errf(line, "base expects <image> [as <name>]")
	}
}

// stageExists reports whether a name refers to an already-declared stage.
func stageExists(p *Plan, name string) bool {
	for _, s := range p.Stages {
		if s.Name == name {
			return true
		}
	}
	return false
}

// looksLikeStageName keeps image references and stage names from colliding.
// Registry references always contain a '.', ':' or '/', none of which are
// legal in a stage name.
func looksLikeStageName(s string) bool {
	if s == "" {
		return false
	}
	if strings.ContainsAny(s, ".:/") {
		return false
	}
	for i, r := range s {
		ok := r == '-' || r == '_' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(i > 0 && r >= '0' && r <= '9')
		if !ok {
			return false
		}
	}
	return true
}

// stripFlags pulls leading --key=value options off an argument list,
// returning the recognised --from value, whether it was present at all, and
// the positional arguments. Options after the first positional argument are
// left alone, so paths that begin with "--" cannot silently disappear.
func stripFlags(args []string) (from string, hasFrom bool, positional []string) {
	positional = args
	for len(positional) > 0 && strings.HasPrefix(positional[0], "--") {
		opt := positional[0]
		positional = positional[1:]
		if strings.HasPrefix(opt, "--from=") {
			from = strings.TrimPrefix(opt, "--from=")
			hasFrom = true
		}
	}
	return from, hasFrom, positional
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
