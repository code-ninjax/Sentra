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
	if len(plan.Stages) != 1 {
		t.Fatalf("stages = %d, want 1", len(plan.Stages))
	}
	stage := plan.Final()
	if stage.Base != "node:20-slim" {
		t.Errorf("Base = %q, want node:20-slim", stage.Base)
	}
	if got, want := len(stage.Steps), 7; got != want {
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
		if stage.Steps[i].Type != w.typ {
			t.Errorf("step %d type = %q, want %q", i, stage.Steps[i].Type, w.typ)
		}
		if got := stage.Steps[i].Summary(); got != w.desc {
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
				if p.Final().Base != "alpine:3.20" {
					t.Errorf("Base = %q", p.Final().Base)
				}
				if p.Final().Steps[0].Type != DirectiveExec {
					t.Errorf("type = %q", p.Final().Steps[0].Type)
				}
			},
		},
		{
			name: "crlf line endings",
			in:   "base alpine\r\nworkdir /app\r\n",
			want: func(t *testing.T, p *Plan) {
				if p.Final().Steps[0].Path != "/app" {
					t.Errorf("path = %q", p.Final().Steps[0].Path)
				}
			},
		},
		{
			name: "quoted exec argument",
			in:   `base alpine` + "\n" + `exec sh -c "echo hi && echo bye"` + "\n",
			want: func(t *testing.T, p *Plan) {
				argv := p.Final().Steps[0].Argv
				if len(argv) != 3 || argv[2] != "echo hi && echo bye" {
					t.Errorf("argv = %q", argv)
				}
			},
		},
		{
			name: "single quotes are literal",
			in:   "base alpine\nexec echo '$HOME'\n",
			want: func(t *testing.T, p *Plan) {
				if got := p.Final().Steps[0].Argv[1]; got != "$HOME" {
					t.Errorf("argv[1] = %q, want $HOME", got)
				}
			},
		},
		{
			name: "escaped space in copy",
			in:   `base alpine` + "\n" + `copy my\ file.txt /app/` + "\n",
			want: func(t *testing.T, p *Plan) {
				if p.Final().Steps[0].Src != "my file.txt" {
					t.Errorf("src = %q", p.Final().Steps[0].Src)
				}
			},
		},
		{
			name: "multiple env pairs on one line",
			in:   "base alpine\nenv A=1 B=2\n",
			want: func(t *testing.T, p *Plan) {
				if got := strings.Join(p.Final().Steps[0].Env, ","); got != "A=1,B=2" {
					t.Errorf("env = %q", got)
				}
			},
		},
		{
			name: "empty env value",
			in:   "base alpine\nenv EMPTY=\n",
			want: func(t *testing.T, p *Plan) {
				if p.Final().Steps[0].Env[0] != "EMPTY=" {
					t.Errorf("env = %q", p.Final().Steps[0].Env[0])
				}
			},
		},
		{
			name: "ports with protocols",
			in:   "base alpine\nexpose 80/tcp 53/udp\n",
			want: func(t *testing.T, p *Plan) {
				if got := p.Final().Steps[0].Summary(); got != "80/tcp 53/udp" {
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
				if p.Final().Steps[0].Cache != "" {
					t.Errorf("step 0 cache = %q, want empty", p.Final().Steps[0].Cache)
				}
				if p.Final().Steps[1].Cache != "deps-v1" {
					t.Errorf("step 1 cache = %q, want deps-v1", p.Final().Steps[1].Cache)
				}
				if p.Final().Steps[2].Cache != "" {
					t.Errorf("step 2 cache = %q, want empty", p.Final().Steps[2].Cache)
				}
			},
		},
		{
			name: "comments and blank lines are ignored",
			in:   "#top\n\nbase alpine\n\n  # indented comment\nexec true\n",
			want: func(t *testing.T, p *Plan) {
				if len(p.Final().Steps) != 1 {
					t.Errorf("steps = %d, want 1", len(p.Final().Steps))
				}
			},
		},
		{
			name: "hash mid-line is literal",
			in:   "base alpine\nexec echo #1\n",
			want: func(t *testing.T, p *Plan) {
				if got := p.Final().Steps[0].Argv[1]; got != "#1" {
					t.Errorf("argv[1] = %q, want #1", got)
				}
			},
		},
		{
			name: "base only is valid",
			in:   "base alpine\n",
			want: func(t *testing.T, p *Plan) {
				if len(p.Final().Steps) != 0 {
					t.Errorf("steps = %d, want 0", len(p.Final().Steps))
				}
			},
		},
		{
			name: "multi-stage build",
			in: "base golang:1.22 as builder\n" +
				"workdir /src\n" +
				"exec go build -o /out/app .\n" +
				"base alpine:3.20\n" +
				"copy --from=builder /out/app /app\n" +
				"start /app\n",
			want: func(t *testing.T, p *Plan) {
				if len(p.Stages) != 2 {
					t.Fatalf("stages = %d, want 2", len(p.Stages))
				}
				if p.Stages[0].Name != "builder" {
					t.Errorf("stage 0 name = %q, want builder", p.Stages[0].Name)
				}
				if p.Stages[0].Base != "golang:1.22" {
					t.Errorf("stage 0 base = %q", p.Stages[0].Base)
				}
				if len(p.Stages[0].Steps) != 2 {
					t.Errorf("stage 0 steps = %d, want 2", len(p.Stages[0].Steps))
				}

				final := p.Final()
				if final.Base != "alpine:3.20" {
					t.Errorf("final base = %q", final.Base)
				}
				if len(final.Steps) != 2 {
					t.Fatalf("final steps = %d, want 2", len(final.Steps))
				}
				if final.Steps[0].From != "builder" {
					t.Errorf("copy from = %q, want builder", final.Steps[0].From)
				}
				if final.Steps[0].Src != "/out/app" || final.Steps[0].Dest != "/app" {
					t.Errorf("copy = %q -> %q", final.Steps[0].Src, final.Steps[0].Dest)
				}
				if g := p.StageNames(); len(g) != 1 || g[0] != "builder" {
					t.Errorf("stage names = %v", g)
				}
			},
		},
		{
			name: "stage inheritance through base",
			in: "base alpine as base\n" +
				"exec touch /marker\n" +
				"base base\n" +
				"exec true\n",
			want: func(t *testing.T, p *Plan) {
				if p.Final().Base != "base" {
					t.Errorf("final base = %q, want the earlier stage name", p.Final().Base)
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
		{"directive before base", "workdir /app\nbase alpine\n", "must come after a base"},
		{"unnamed intermediate stage", "base alpine\nbase busybox\n", "every stage except the last must be named"},
		{"base no image", "base\n", "base expects <image>"},
		{"base too many args", "base alpine extra\n", "base expects <image>"},
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
		{"copy from unknown stage", "base alpine\ncopy --from=nope a b\n", "unknown stage"},
		{"copy from missing value", "base alpine as s\nbase busybox\ncopy --from= a b\n", "needs a stage name"},
		{"duplicate stage name", "base alpine as build\nbase busybox as build\n", "duplicate stage name"},
		{"bad stage name", "base alpine as 9bad\n", "invalid stage name"},
		{"base as without name", "base alpine as\n", "base expects <image>"},
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
