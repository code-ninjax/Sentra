package build

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"sentra/internal/config"
)

func steps(spec ...config.Step) []config.Step { return spec }

func TestDepsExecSerializesOnFilesystem(t *testing.T) {
	g := NewGraph(steps(
		config.Step{Type: config.DirectiveCopy, Src: "a", Dest: "/a"},
		config.Step{Type: config.DirectiveExec, Argv: []string{"build"}},
		config.Step{Type: config.DirectiveExec, Argv: []string{"test"}},
	))

	if got := g.NodeAt(0).Deps; len(got) != 0 {
		t.Errorf("first step should have no deps, got %v", got)
	}
	if got := g.NodeAt(1).Deps; len(got) != 1 || got[0] != 0 {
		t.Errorf("exec deps = %v, want [0]", got)
	}
	if got := g.NodeAt(2).Deps; len(got) != 2 {
		t.Errorf("second exec deps = %v, want both prior writers", got)
	}
}

func TestDepsDisjointCopiesAreParallel(t *testing.T) {
	g := NewGraph(steps(
		config.Step{Type: config.DirectiveCopy, Src: "a", Dest: "/app/a"},
		config.Step{Type: config.DirectiveCopy, Src: "b", Dest: "/app/b"},
		config.Step{Type: config.DirectiveCopy, Src: "c", Dest: "/opt/c"},
	))
	for i := 1; i < 3; i++ {
		if got := g.NodeAt(i).Deps; len(got) != 0 {
			t.Errorf("copy %d should be independent, got deps %v", i, got)
		}
	}
}

func TestDepsOverlappingCopiesSerialize(t *testing.T) {
	g := NewGraph(steps(
		config.Step{Type: config.DirectiveCopy, Src: "a", Dest: "/app"},
		config.Step{Type: config.DirectiveCopy, Src: "b", Dest: "/app/src"},
		config.Step{Type: config.DirectiveCopy, Src: "c", Dest: "/application"},
		config.Step{Type: config.DirectiveCopy, Src: "d", Dest: "/app"},
	))

	if got := g.NodeAt(1).Deps; len(got) != 1 || got[0] != 0 {
		t.Errorf("nested dest deps = %v, want [0]", got)
	}
	if got := g.NodeAt(2).Deps; len(got) != 0 {
		t.Errorf("/application must not overlap /app, got %v", got)
	}
	if got := g.NodeAt(3).Deps; len(got) != 2 {
		t.Errorf("repeated dest deps = %v, want both earlier writers", got)
	}
}

func TestDestsOverlap(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"/app", "/app", true},
		{"/app", "/app/", true},
		{"/app", "/app/src", true},
		{"/app/src", "/app", true},
		{"/app", "/application", false},
		{".", "/app", true},
		{"/app", ".", true},
		{"/a/b/c", "/a", true},
		{"/a", "/b", false},
	}
	for _, c := range cases {
		if got := destsOverlap(c.a, c.b); got != c.want {
			t.Errorf("destsOverlap(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestMetadataStepsAreIndependent(t *testing.T) {
	g := NewGraph(steps(
		config.Step{Type: config.DirectiveEnv, Env: []string{"A=1"}},
		config.Step{Type: config.DirectiveExpose, Ports: []string{"80"}},
		config.Step{Type: config.DirectiveStart, Argv: []string{"run"}},
	))
	for i := 0; i < 3; i++ {
		if got := g.NodeAt(i).Deps; len(got) != 0 {
			t.Errorf("step %d (%s) should be independent, got %v", i, g.NodeAt(i).Step.Type, got)
		}
	}
}

func TestEnvStepsAreIndependent(t *testing.T) {
	g := NewGraph(steps(
		config.Step{Type: config.DirectiveEnv, Env: []string{"A=1"}},
		config.Step{Type: config.DirectiveEnv, Env: []string{"B=2"}},
	))
	for i := 0; i < 2; i++ {
		if got := g.NodeAt(i).Deps; len(got) != 0 {
			t.Errorf("env step %d should be independent, got %v", i, got)
		}
	}
}

// Later steps see earlier env values through their snapshot, so env
// directives need no ordering barrier between them.
func TestEnvSnapshotAccumulates(t *testing.T) {
	g := NewGraph(steps(
		config.Step{Type: config.DirectiveEnv, Env: []string{"A=1"}},
		config.Step{Type: config.DirectiveExec, Argv: []string{"x"}},
		config.Step{Type: config.DirectiveEnv, Env: []string{"A=2", "B=3"}},
		config.Step{Type: config.DirectiveExec, Argv: []string{"y"}},
	))

	first := strings.Join(g.NodeAt(1).State.Env, ",")
	if first != "A=1" {
		t.Errorf("step 1 env = %q, want A=1", first)
	}
	second := strings.Join(g.NodeAt(3).State.Env, ",")
	if second != "A=2,B=3" {
		t.Errorf("step 3 env = %q, want A=2,B=3 (later key wins)", second)
	}
}

// A workdir does not touch the filesystem, so it carries no dependency —
// and critically, it cannot retarget an earlier COPY that already captured
// the previous workdir in its snapshot. That is what makes running them
// concurrently safe.
func TestWorkdirSnapshotsAreOrderIndependent(t *testing.T) {
	g := NewGraph(steps(
		config.Step{Type: config.DirectiveCopy, Src: "a", Dest: "/a"},
		config.Step{Type: config.DirectiveWorkdir, Path: "/app"},
		config.Step{Type: config.DirectiveCopy, Src: "b", Dest: "b"},
	))

	if got := g.NodeAt(1).Deps; len(got) != 0 {
		t.Errorf("workdir should be independent, got %v", got)
	}
	if got := g.NodeAt(2).Deps; len(got) != 0 {
		t.Errorf("copy after workdir should be independent, got %v", got)
	}

	if got := g.NodeAt(0).State.Workdir; got != "" {
		t.Errorf("copy before workdir sees workdir %q, want empty", got)
	}
	if got := g.NodeAt(2).State.Workdir; got != "/app" {
		t.Errorf("copy after workdir sees workdir %q, want /app", got)
	}
}

// An exec runs after every earlier filesystem writer, whether or not that
// writer was a copy, and sees the workdir in effect where it was written.
func TestExecDependsOnAllPriorWriters(t *testing.T) {
	g := NewGraph(steps(
		config.Step{Type: config.DirectiveCopy, Src: "a", Dest: "/a"},
		config.Step{Type: config.DirectiveWorkdir, Path: "/app"},
		config.Step{Type: config.DirectiveExec, Argv: []string{"uses-cwd"}},
	))

	got := g.NodeAt(2).Deps
	if len(got) != 1 || got[0] != 0 {
		t.Errorf("exec deps = %v, want [0]", got)
	}
	if wd := g.NodeAt(2).State.Workdir; wd != "/app" {
		t.Errorf("exec workdir = %q, want /app", wd)
	}
}

func TestFinalSnapshot(t *testing.T) {
	g := NewGraph(steps(
		config.Step{Type: config.DirectiveEnv, Env: []string{"PORT=3000"}},
		config.Step{Type: config.DirectiveExpose, Ports: []string{"3000"}},
		config.Step{Type: config.DirectiveStart, Argv: []string{"npm", "start"}},
	))
	final := g.Final()
	if strings.Join(final.Env, ",") != "PORT=3000" {
		t.Errorf("final env = %v", final.Env)
	}
	if strings.Join(final.Expose, ",") != "3000" {
		t.Errorf("final expose = %v", final.Expose)
	}
	if strings.Join(final.Entrypoint, " ") != "npm start" {
		t.Errorf("final entrypoint = %v", final.Entrypoint)
	}
}

func TestOrderIsTopological(t *testing.T) {
	all := steps(
		config.Step{Type: config.DirectiveCopy, Src: "a", Dest: "/a"},
		config.Step{Type: config.DirectiveCopy, Src: "b", Dest: "/b"},
		config.Step{Type: config.DirectiveExec, Argv: []string{"go"}},
		config.Step{Type: config.DirectiveCopy, Src: "c", Dest: "/c"},
		config.Step{Type: config.DirectiveExpose, Ports: []string{"80"}},
	)
	g := NewGraph(all)
	order, err := g.Order()
	if err != nil {
		t.Fatalf("Order: %v", err)
	}
	if len(order) != len(all) {
		t.Fatalf("order has %d entries, want %d", len(order), len(all))
	}

	position := map[int]int{}
	for pos, idx := range order {
		position[idx] = pos
	}
	for _, n := range g.nodes {
		for _, d := range n.Deps {
			if position[d] > position[n.Index] {
				t.Errorf("step %d ran before its dependency %d", n.Index, d)
			}
		}
	}
}

func TestRunConcurrentRunsEveryStep(t *testing.T) {
	g := NewGraph(steps(
		config.Step{Type: config.DirectiveCopy, Src: "a", Dest: "/a"},
		config.Step{Type: config.DirectiveCopy, Src: "b", Dest: "/b"},
		config.Step{Type: config.DirectiveExec, Argv: []string{"x"}},
		config.Step{Type: config.DirectiveExpose, Ports: []string{"80"}},
	))

	var mu sync.Mutex
	seen := map[int]bool{}
	err := g.RunConcurrent(4, func(n Node) error {
		mu.Lock()
		seen[n.Index] = true
		mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatalf("RunConcurrent: %v", err)
	}
	if len(seen) != g.Len() {
		t.Errorf("ran %d of %d steps", len(seen), g.Len())
	}
}

func TestRunConcurrentRespectsDependencies(t *testing.T) {
	g := NewGraph(steps(
		config.Step{Type: config.DirectiveCopy, Src: "a", Dest: "/a"},
		config.Step{Type: config.DirectiveExec, Argv: []string{"needs-a"}},
		config.Step{Type: config.DirectiveCopy, Src: "c", Dest: "/app/c"},
	))

	var (
		mu       sync.Mutex
		finished = map[int]bool{}
		firstErr error
	)
	err := g.RunConcurrent(4, func(n Node) error {
		mu.Lock()
		defer mu.Unlock()
		for _, d := range g.NodeAt(n.Index).Deps {
			if !finished[d] {
				firstErr = fmt.Errorf("step %d ran before dependency %d finished", n.Index, d)
				return firstErr
			}
		}
		time.Sleep(time.Millisecond)
		finished[n.Index] = true
		return nil
	})
	if err != nil {
		t.Fatalf("RunConcurrent: %v", err)
	}
}

func TestRunConcurrentReportsFirstFailure(t *testing.T) {
	g := NewGraph(steps(
		config.Step{Type: config.DirectiveCopy, Src: "a", Dest: "/a"},
		config.Step{Type: config.DirectiveCopy, Src: "b", Dest: "/b"},
	))
	var attempts int32
	err := g.RunConcurrent(2, func(n Node) error {
		atomic.AddInt32(&attempts, 1)
		return fmt.Errorf("boom")
	})
	if err == nil {
		t.Fatal("RunConcurrent returned nil despite failing steps")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %q, want it to wrap the step failure", err)
	}
}

func TestRunConcurrentSingleWorkerIsSequential(t *testing.T) {
	g := NewGraph(steps(
		config.Step{Type: config.DirectiveCopy, Src: "a", Dest: "/a"},
		config.Step{Type: config.DirectiveCopy, Src: "b", Dest: "/b"},
		config.Step{Type: config.DirectiveCopy, Src: "c", Dest: "/c"},
	))
	var order []int
	err := g.RunConcurrent(1, func(n Node) error {
		order = append(order, n.Index)
		return nil
	})
	if err != nil {
		t.Fatalf("RunConcurrent: %v", err)
	}
	for i, got := range order {
		if got != i {
			t.Fatalf("single worker ran out of order: %v", order)
		}
	}
}
