package build

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"sentra/internal/config"
)

// Snapshot is the build state visible to a step: the workdir in effect and
// the environment, exposed ports and entrypoint accumulated by directives
// before it.
//
// Steps read their own snapshot instead of shared executor fields. That is
// what makes concurrent execution safe: a WORKDIR running beside an earlier
// COPY can no longer change where that COPY puts its files, because the COPY
// already knows the workdir it was written with.
type Snapshot struct {
	Workdir    string
	Env        []string
	Expose     []string
	Entrypoint []string
}

// Node is one build step, the indices it must wait for, and the build
// state it sees when it runs.
type Node struct {
	Index int
	Step  config.Step
	Deps  []int
	State Snapshot
}

// Graph is the dependency DAG for a Sentrafile's build steps, plus the
// state the finished image carries.
//
// Sentrafiles read top-to-bottom, so a naive executor just runs them in
// file order. That is always correct and often not optimal: two `copy`
// steps writing to disjoint destinations share no dependency and can run
// at the same time, and `expose`/`start` touch the filesystem not at all.
//
// The dependency rules are deliberately conservative — a step runs in
// parallel only when we can prove it cannot observe the other's writes:
//
//	copy     depends on any earlier copy whose destination overlaps its own
//	exec     depends on every earlier filesystem writer: an arbitrary
//	         command can read or write anything in the rootfs
//	workdir  independent (it resolves destinations through snapshots)
//	env      independent
//	expose   independent
//	start    independent
//
// exec stays sequential by design. Running two arbitrary commands against
// one mutable rootfs is a data race, not a speedup, so the graph never
// claims those steps are independent.
type Graph struct {
	nodes []Node
	final Snapshot
}

// NewGraph derives each step's dependencies and state snapshot.
//
// Snapshots come from a single pass in file order, so the state a step sees
// is a property of the Sentrafile rather than of scheduling.
func NewGraph(steps []config.Step) *Graph {
	g := &Graph{nodes: make([]Node, len(steps))}

	state := Snapshot{}
	var priorCopies []int
	var writers []int

	for i, step := range steps {
		n := Node{Index: i, Step: step}

		switch step.Type {
		case config.DirectiveWorkdir:
			// A workdir changes how later destinations resolve, not what
			// is on disk, so it carries no filesystem dependency.
			state.Workdir = step.Path

		case config.DirectiveExec:
			n.Deps = append([]int(nil), writers...)
			writers = append(writers, i)

		case config.DirectiveCopy:
			for _, j := range priorCopies {
				if destsOverlap(steps[j].Dest, step.Dest) {
					n.Deps = append(n.Deps, j)
				}
			}
			priorCopies = append(priorCopies, i)
			writers = append(writers, i)

		case config.DirectiveEnv:
			state.Env = mergeEnv(state.Env, step.Env)

		case config.DirectiveExpose:
			state.Expose = append(state.Expose, step.Ports...)

		case config.DirectiveStart:
			state.Entrypoint = step.Argv
		}

		n.State = state
		g.nodes[i] = n
	}
	g.final = state
	return g
}

// Final is the state the built image carries.
func (g *Graph) Final() Snapshot { return g.final }

// destsOverlap reports whether two copy destinations can touch the same
// path. Comparison is component-wise, so "app" overlaps "app/src" but
// "application" does not. Both relative and absolute destinations are
// normalized against the build root first.
func destsOverlap(a, b string) bool {
	na, nb := normalizeDest(a), normalizeDest(b)
	if na == nb {
		return true
	}
	if na == "/" || nb == "/" {
		return true
	}
	return strings.HasPrefix(nb, na+"/") || strings.HasPrefix(na, nb+"/")
}

// normalizeDest expresses a destination as an absolute path inside the
// build root, so "app/x" and "/app/x" compare equal.
func normalizeDest(dest string) string {
	clean := path.Clean(filepath.ToSlash(dest))
	if clean == "." || clean == "/" {
		return "/"
	}
	return "/" + strings.Trim(clean, "/")
}

// Len reports the node count.
func (g *Graph) Len() int { return len(g.nodes) }

// NodeAt returns the node for a Sentrafile step index.
func (g *Graph) NodeAt(i int) Node { return g.nodes[i] }

// Order returns a topologically valid execution order. Steps with no
// dependency between them come out in file order, so builds are
// reproducible run to run.
func (g *Graph) Order() ([]int, error) {
	indegree := make([]int, len(g.nodes))
	dependents := make([][]int, len(g.nodes))
	for _, n := range g.nodes {
		indegree[n.Index] = len(n.Deps)
		for _, d := range n.Deps {
			dependents[d] = append(dependents[d], n.Index)
		}
	}

	var ready []int
	for i, deg := range indegree {
		if deg == 0 {
			ready = append(ready, i)
		}
	}
	sort.Ints(ready)

	var order []int
	for len(ready) > 0 {
		n := ready[0]
		ready = ready[1:]
		order = append(order, n)

		var unlocked []int
		for _, d := range dependents[n] {
			indegree[d]--
			if indegree[d] == 0 {
				unlocked = append(unlocked, d)
			}
		}
		if len(unlocked) > 0 {
			ready = append(ready, unlocked...)
			sort.Ints(ready)
		}
	}

	if len(order) != len(g.nodes) {
		return nil, errors.New("build graph contains a dependency cycle")
	}
	return order, nil
}

// Independent reports the indices that could run concurrently with n.
// Metadata-only steps are always independent; copy steps are independent
// of everything the graph did not name as a dependency.
func (g *Graph) Independent(n Node) []int {
	blocked := make(map[int]bool, len(n.Deps)+1)
	for _, d := range n.Deps {
		blocked[d] = true
	}
	blocked[n.Index] = true

	var out []int
	for _, other := range g.nodes {
		if blocked[other.Index] {
			continue
		}
		if overlapsAny(n, other) {
			continue
		}
		out = append(out, other.Index)
	}
	return out
}

func overlapsAny(a, b Node) bool {
	if a.Step.Type == config.DirectiveCopy && b.Step.Type == config.DirectiveCopy {
		return destsOverlap(a.Step.Dest, b.Step.Dest)
	}
	// Anything that can touch the filesystem conflicts with anything else
	// that can touch the filesystem.
	return mutates(a.Step.Type) && mutates(b.Step.Type)
}

func mutates(d config.Directive) bool {
	switch d {
	case config.DirectiveExec, config.DirectiveCopy, config.DirectiveWorkdir:
		return true
	}
	return false
}

// RunConcurrent executes fn for every node with at most workers running
// at once, starting a node only once all of its dependencies have
// succeeded. fn must be safe for concurrent use. The first failure
// aborts the build; the returned error names the lowest failing step.
func (g *Graph) RunConcurrent(workers int, fn func(n Node) error) error {
	if workers < 1 {
		workers = 1
	}
	if workers > g.Len() {
		workers = g.Len()
	}
	if workers < 1 {
		return nil
	}
	if _, err := g.Order(); err != nil {
		return err
	}

	var (
		mu        sync.Mutex
		cond      = sync.NewCond(&mu)
		remaining = make([]int, 0, g.Len())
		done      = make([]bool, g.Len())
		active    int
		firstErr  error
	)
	for i := range g.nodes {
		remaining = append(remaining, i)
	}

	var wg sync.WaitGroup
	worker := func() {
		defer wg.Done()
		for {
			mu.Lock()
			picked := -1
		pick:
			for {
				if firstErr != nil || len(remaining) == 0 {
					break pick
				}
				for k, n := range remaining {
					if readyLocked(g, n, done) {
						picked = k
						break pick
					}
				}
				if active == 0 {
					firstErr = errors.New("build graph contains a dependency cycle")
					break pick
				}
				cond.Wait()
			}
			if picked < 0 {
				cond.Broadcast()
				mu.Unlock()
				return
			}

			n := remaining[picked]
			remaining = append(remaining[:picked], remaining[picked+1:]...)
			active++
			mu.Unlock()

			err := fn(g.nodes[n])

			mu.Lock()
			active--
			switch {
			case err != nil && firstErr == nil:
				firstErr = fmt.Errorf("step %d (%s): %w", n, g.nodes[n].Step.Type, err)
			case err == nil:
				done[n] = true
			}
			cond.Broadcast()
			mu.Unlock()
		}
	}

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go worker()
	}
	wg.Wait()
	return firstErr
}

func readyLocked(g *Graph, n int, done []bool) bool {
	if n < 0 || n >= len(g.nodes) || done[n] {
		return false
	}
	for _, d := range g.nodes[n].Deps {
		if !done[d] {
			return false
		}
	}
	return true
}
