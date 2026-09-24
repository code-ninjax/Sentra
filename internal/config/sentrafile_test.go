package config

import (
	"strings"
	"testing"
)

const nodeExample = `# Node service
base node:20-slim

workdir /app
copy package.json .
exec npm install
copy . .

env PORT=3000
expose 3000

start npm run start
`

func TestParseNodeExample(t *testing.T) {
	plan, err := Parse([]byte(nodeExample))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if plan.Base != "node:20-slim" {
		t.Errorf("Base = %q, want node:20-slim", plan.Base)
	}
	if got, want := len(plan.Steps), 7; got != want {
		t.Fatalf("steps = %d, want %d", got, want)
	}

	want := []struct {
		typ  Directive
		desc string
	}{
		{DirectiveWorkdir, "/app"},
		{DirectiveCopy, "package.json -> ."},
		{DirectiveExec, "npm install"},
		{DirectiveCopy, ". -> ."},
		{DirectiveEnv, "PORT=3000"},
		{DirectiveExpose, "3000"},
		{DirectiveStart, "npm run start"},
	}
	for i, w := range want {
		if plan.Steps[i].Type != w.typ {
			t.Errorf("step %d type = %q, want %q", i, plan.Steps[i].Type, w.typ)
		}
		if got := plan.Steps[i].Summary(); got != w.desc {
			t.Errorf("step %d summary = %q, want %q", i, got, w.desc)
		}
	}

	if plan.Security != DefaultSecurity() {
		t.Errorf("security = %+v, want defaults %+v", plan.Security, DefaultSecurity())
	}
}

func TestParseAccepts(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want func(*testing.T, *Plan)
	}{
		{
			name: "directives are case insensitive",
			in:   "BASE alpine:3.20\nEXEC echo hi\n",
			want: func(t *testing.T, p *Plan) {
				if p.Base != "alpine:3.20" {
					t.Errorf("Base = %q", p.Base)
				}
				if p.Steps[0].Type != DirectiveExec {
					t.Errorf("type = %q", p.Steps[0].Type)
				}
			},
		},
		{
			name: "crlf line endings",
			in:   "base alpine\r\nworkdir /app\r\n",
			want: func(t *testing.T, p *Plan) {
				if p.Steps[0].Path != "/app" {
					t.Errorf("path = %q", p.Steps[0].Path)
				}
			},
		},
		{
			name: "quoted exec argument",
			in:   `base alpine` + "\n" + `exec sh -c "echo hi && echo bye"` + "\n",
			want: func(t *testing.T, p *Plan) {
				argv := p.Steps[0].Argv
				if len(argv) != 3 || argv[2] != "echo hi && echo bye" {
					t.Errorf("argv = %q", argv)
				}
			},
		},
		{
			name: "single quotes are literal",
			in:   "base alpine\nexec echo '$HOME'\n",
			want: func(t *testing.T, p *Plan) {
				if got := p.Steps[0].Argv[1]; got != "$HOME" {
					t.Errorf("argv[1] = %q, want $HOME", got)
				}
			},
		},
		{
			name: "escaped space in copy",
			in:   `base alpine` + "\n" + `copy my\ file.txt /app/` + "\n",
			want: func(t *testing.T, p *Plan) {
				if p.Steps[0].Src != "my file.txt" {
					t.Errorf("src = %q", p.Steps[0].Src)
				}
			},
		},
		{
			name: "multiple env pairs on one line",
			in:   "base alpine\nenv A=1 B=2\n",
			want: func(t *testing.T, p *Plan) {
				if got := strings.Join(p.Steps[0].Env, ","); got != "A=1,B=2" {
					t.Errorf("env = %q", got)
				}
			},
		},
		{
			name: "empty env value",
			in:   "base alpine\nenv EMPTY=\n",
			want: func(t *testing.T, p *Plan) {
				if p.Steps[0].Env[0] != "EMPTY=" {
					t.Errorf("env = %q", p.Steps[0].Env[0])
				}
			},
		},
		{
			name: "ports with protocols",
			in:   "base alpine\nexpose 80/tcp 53/udp\n",
			want: func(t *testing.T, p *Plan) {
				if got := p.Steps[0].Summary(); got != "80/tcp 53/udp" {
					t.Errorf("ports = %q", got)
				}
			},
		},
		{
			name: "secure posture",
			in:   "base alpine\nsecure rootless=false readonly=true seccomp=strict\n",
			want: func(t *testing.T, p *Plan) {
				if p.Security.Rootless || !p.Security.Readonly || p.Security.Seccomp != "strict" {
					t.Errorf("security = %+v", p.Security)
				}
			},
		},
		{
			name: "cache key attaches to the next step",
			in:   "base alpine\ncopy a b\ncache key=deps-v1\ncopy c d\nexec make\n",
			want: func(t *testing.T, p *Plan) {
				if p.Steps[0].Cache != "" {
					t.Errorf("step 0 cache = %q, want empty", p.Steps[0].Cache)
				}
				if p.Steps[1].Cache != "deps-v1" {
					t.Errorf("step 1 cache = %q, want deps-v1", p.Steps[1].Cache)
				}
				if p.Steps[2].Cache != "" {
					t.Errorf("step 2 cache = %q, want empty", p.Steps[2].Cache)
				}
			},
		},
		{
			name: "comments and blank lines are ignored",
			in:   "#top\n\nbase alpine\n\n  # indented comment\nexec true\n",
			want: func(t *testing.T, p *Plan) {
				if len(p.Steps) != 1 {
					t.Errorf("steps = %d, want 1", len(p.Steps))
				}
			},
		},
		{
			name: "hash mid-line is literal",
			in:   "base alpine\nexec echo #1\n",
			want: func(t *testing.T, p *Plan) {
				if got := p.Steps[0].Argv[1]; got != "#1" {
					t.Errorf("argv[1] = %q, want #1", got)
				}
			},
		},
		{
			name: "base only is valid",
			in:   "base alpine\n",
			want: func(t *testing.T, p *Plan) {
				if len(p.Steps) != 0 {
					t.Errorf("steps = %d, want 0", len(p.Steps))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := Parse([]byte(tt.in))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			tt.want(t, plan)
		})
	}
}

func TestParseRejects(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr string
	}{
		{"empty file", "", "missing base"},
		{"unknown directive", "base alpine\nfrobnicate x\n", "unknown directive"},
		{"base not first", "workdir /app\nbase alpine\n", "must be the first"},
		{"duplicate base", "base alpine\nbase busybox\n", "duplicate base"},
		{"base no image", "base\n", "exactly one image"},
		{"base too many args", "base alpine extra\n", "exactly one image"},
		{"relative workdir", "base alpine\nworkdir app\n", "must be absolute"},
		{"copy one arg", "base alpine\ncopy only\n", "copy requires"},
		{"copy three args", "base alpine\ncopy a b c\n", "copy requires"},
		{"exec no command", "base alpine\nexec\n", "exec requires"},
		{"env no pairs", "base alpine\nenv\n", "at least one KEY=VALUE"},
		{"env missing equals", "base alpine\nenv PORT\n", "expects KEY=VALUE"},
		{"env empty key", "base alpine\nenv =3000\n", "expects KEY=VALUE"},
		{"expose no ports", "base alpine\nexpose\n", "at least one port"},
		{"expose not a number", "base alpine\nexpose http\n", "invalid port"},
		{"expose out of range", "base alpine\nexpose 70000\n", "invalid port"},
		{"expose bad proto", "base alpine\nexpose 80/quic\n", "invalid port"},
		{"duplicate start", "base alpine\nstart a\nstart b\n", "duplicate start"},
		{"start no command", "base alpine\nstart\n", "start requires"},
		{"cache wrong form", "base alpine\ncache deps\n", "key=<value>"},
		{"cache no value", "base alpine\ncache key=\n", "key=<value>"},
		{"cache trailing", "base alpine\ncache key=v1\n", "must be followed"},
		{"cache back to back", "base alpine\ncache key=a\ncache key=b\n", "must be followed by a step"},
		{"secure no args", "base alpine\nsecure\n", "at least one of"},
		{"secure bad bool", "base alpine\nsecure rootless=yes\n", "expects a bool"},
		{"secure bad seccomp", "base alpine\nsecure seccomp=paranoid\n", "default|strict"},
		{"secure unknown key", "base alpine\nsecure caps=all\n", "unknown secure option"},
		{"duplicate secure", "base alpine\nsecure readonly=false\nsecure readonly=true\n", "duplicate secure"},
		{"unterminated quote", "base alpine\nexec sh -c \"oops\n", "unterminated double quote"},
		{"unterminated single quote", "base alpine\nexec 'oops\n", "unterminated single quote"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.in))
			if err == nil {
				t.Fatalf("Parse succeeded, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", err, tt.wantErr)
			}
			var perr *Error
			if !asError(err, &perr) {
				t.Fatalf("error type = %T, want *config.Error", err)
			}
			if perr.Line < 1 {
				t.Errorf("error line = %d, want >= 1", perr.Line)
			}
		})
	}
}

func TestParseFileMissing(t *testing.T) {
	if _, err := ParseFile("testdata/does-not-exist"); err == nil {
		t.Fatal("ParseFile succeeded for a missing file")
	}
}

func asError(err error, target **Error) bool {
	e, ok := err.(*Error)
	if ok {
		*target = e
	}
	return ok
}
