package build

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"sentra/internal/config"
	"sentra/internal/image"
	"sentra/internal/overlayfs"
	rt "sentra/internal/runtime"
)

// Options configure a build.
type Options struct {
	ContextDir string    // directory holding the Sentrafile and build inputs
	Sentrafile string    // path to the Sentrafile (default: <ContextDir>/Sentrafile)
	Tag        string    // output image tag
	NoCache    bool      // ignore cached layers and re-run every step
	Workers    int       // parallel step workers (0 = auto)
	Out        io.Writer // progress and step output
}

// StepResult records what happened to one step.
type StepResult struct {
	Index     int              `json:"index"`
	Directive config.Directive `json:"directive"`
	CacheHit  bool             `json:"cache_hit"`
	ManualKey bool             `json:"manual_key"`
	CacheKey  string           `json:"cache_key,omitempty"`
	Duration  time.Duration    `json:"duration"`
}

// Result describes a completed build.
type Result struct {
	Tag        string
	Base       string
	Rootfs     string
	Env        []string
	Expose     []string
	Entrypoint []string
	Security   config.Security
	Steps      []StepResult
	Duration   time.Duration
}

// Image is the persisted description of a built image.
type Image struct {
	Tag        string          `json:"tag"`
	Base       string          `json:"base"`
	Rootfs     string          `json:"rootfs"`
	Env        []string        `json:"env,omitempty"`
	Expose     []string        `json:"expose,omitempty"`
	Entrypoint []string        `json:"entrypoint,omitempty"`
	Security   config.Security `json:"security"`
	Layers     []string        `json:"layers"`
	// BuildLayers are the layers this build produced, excluding the base
	// image's own layers. Push uploads only these, because the base is
	// already in the registry it came from.
	BuildLayers []string     `json:"build_layers,omitempty"`
	Fallback    bool         `json:"fallback,omitempty"`
	Steps       []StepResult `json:"steps"`
	BuiltAt     time.Time    `json:"built_at"`
}

// layer is one entry in the overlay chain: where its files live, the
// content identity used to derive the next step's cache key, and the step
// index that produced it. Index ordering is what keeps concurrent steps
// stacking in Sentrafile order rather than completion order.
type layer struct {
	Path  string
	Key   string
	Index int
}

type executor struct {
	opts      Options
	plan      *config.Plan
	cache     *Cache
	stateRoot string
	out       io.Writer

	mu        sync.Mutex
	chain     []layer
	plainRoot string // non-layered fallback: one writable copy of the base
	results   map[int]StepResult
	layered   bool
	buildID   string
	baseCount int // number of chain entries contributed by this stage's base

	stageChains map[string][]layer // stage name -> its full layer chain
	stageRoots  map[string]string  // stage name -> merged rootfs, for copy --from=
	finalStage  config.Stage
	stepBase    int // steps executed before the current stage began
	ranSteps    int // total steps executed so far

	final Snapshot // workdir/env/ports/entrypoint of the finished image
}

// Build executes a Sentrafile and produces a runnable image rootfs.
//
// The execution model is overlayfs-native: the base image is a read-only
// lowerdir and every mutating step gets a fresh upperdir mounted over the
// accumulated chain. Whatever a step writes lands in that upperdir, which
// is exactly the diff worth caching — there is no tar/untar round-trip
// anywhere in the path, and a cached step costs a directory entry rather
// than a filesystem copy.
func Build(opts Options) (*Result, error) {
	if opts.Out == nil {
		opts.Out = os.Stdout
	}
	if opts.Workers <= 0 {
		// Builds are I/O-bound — hashing sources, copying files, mounting
		// overlays — so a couple of workers past the core count still pays
		// off, which is what makes a one-core WSL guest parallelize at all.
		// The cap of four keeps concurrent container starts from thrashing
		// a small machine.
		opts.Workers = runtime.NumCPU()
		if opts.Workers < 2 {
			opts.Workers = 2
		}
		if opts.Workers > 4 {
			opts.Workers = 4
		}
	}
	if opts.ContextDir == "" {
		opts.ContextDir = "."
	}
	if opts.Sentrafile == "" {
		opts.Sentrafile = filepath.Join(opts.ContextDir, "Sentrafile")
	}

	started := time.Now()

	plan, err := config.ParseFile(opts.Sentrafile)
	if err != nil {
		return nil, err
	}
	stateRoot, err := rt.StateRoot()
	if err != nil {
		return nil, err
	}
	cache, err := NewCache(stateRoot, !opts.NoCache)
	if err != nil {
		return nil, err
	}

	ex := &executor{
		opts:        opts,
		plan:        plan,
		cache:       cache,
		stateRoot:   stateRoot,
		out:         opts.Out,
		results:     map[int]StepResult{},
		stageChains: map[string][]layer{},
		stageRoots:  map[string]string{},
		// An uncacheable command makes subsequent automatic cache keys unique
		// for this build, preventing a copy cache entry from being reused on
		// an unknown command result.
		buildID: fmt.Sprintf("%d", time.Now().UnixNano()),
	}

	ex.enableLayering()

	// Stages run in declaration order and share one layer cache, so an
	// unchanged stage costs a directory lookup rather than a rebuild. Only
	// the final stage becomes the image; earlier ones are kept as merged
	// roots so `copy --from=` can read from them.
	ran := 0
	for si, stage := range plan.Stages {
		last := si == len(plan.Stages)-1
		label := stage.Name
		if label == "" {
			label = fmt.Sprintf("stage %d", si+1)
		}
		if len(plan.Stages) > 1 {
			fmt.Fprintf(ex.out, "\n%s (%s)\n", label, stage.Base)
		}

		// Step results are keyed globally, so a stage's local step indices
		// are offset by the steps that came before it.
		ex.stepBase = ran
		ran += len(stage.Steps)

		if err := ex.prepareStage(stage); err != nil {
			return nil, err
		}
		if err := ex.run(stage); err != nil {
			return nil, err
		}
		ex.final = NewGraph(stage.Steps).Final()
		ex.finalStage = stage

		if err := ex.closeStage(stage, last); err != nil {
			return nil, err
		}
	}
	_ = cache.SaveHashIndex()

	img, err := ex.assembleImage()
	if err != nil {
		return nil, err
	}
	if err := ex.writeImageRecord(img); err != nil {
		return nil, err
	}

	return &Result{
		Tag:        img.Tag,
		Base:       img.Base,
		Rootfs:     img.Rootfs,
		Env:        ex.final.Env,
		Expose:     ex.final.Expose,
		Entrypoint: ex.final.Entrypoint,
		Security:   plan.Security,
		Steps:      ex.orderedResults(),
		Duration:   time.Since(started),
	}, nil
}

// enableLayering probes whether per-step overlayfs upperdirs work here.
// Failure is not fatal: the build degrades to a single writable rootfs
// copy with no layer caching, which is slow but correct.
func (ex *executor) enableLayering() {
	work := filepath.Join(ex.stateRoot, "build-work")
	lower := filepath.Join(work, "probe-lower")
	upper := filepath.Join(work, "probe-upper")
	workdir := filepath.Join(work, "probe-work")
	mountpoint := filepath.Join(work, "probe-mount")

	for _, d := range []string{lower, upper, workdir, mountpoint} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			ex.warnLayering(fmt.Errorf("create %s: %w", d, err))
			return
		}
	}
	if err := overlayfs.Probe(lower, upper, workdir, mountpoint); err != nil {
		ex.warnLayering(err)
		return
	}
	ex.layered = true
}

func (ex *executor) warnLayering(err error) {
	fmt.Fprintf(ex.out, "note: per-step layer caching unavailable (%v)\n", err)
	fmt.Fprintf(ex.out, "      building without cache; run as root for fast rebuilds\n")
}

// prepareBase resolves the base image and seeds the overlay chain.
//
// The chain starts from the base image's extracted layer directories, not
// from its merged rootfs. A merged rootfs is itself an overlay mount, and
// using a live overlay as an overlayfs lowerdir produces a merged view
// that is missing the base contents. Plain layer directories also mean a
// rootful build never copies the base filesystem to get started.
//
// A stage either starts from a registry image or inherits an earlier
// stage's layer chain, which is what makes `base builder` free.
func (ex *executor) prepareStage(stage config.Stage) error {
	ex.plainRoot = ""

	if chain, ok := ex.stageChains[stage.Base]; ok {
		ex.chain = append([]layer(nil), chain...)
		ex.baseCount = len(ex.chain)
		if !ex.layered {
			if err := ex.preparePlainRoot(ex.chain); err != nil {
				return err
			}
			ex.baseCount = 0
		}
		return nil
	}

	if len(ex.plan.Stages) == 1 {
		fmt.Fprintf(ex.out, "base %s\n", stage.Base)
	}

	// Pull keeps the record and the merged rootfs up to date; the build
	// consumes the layers it wrote.
	if _, err := image.Pull(stage.Base); err != nil {
		return fmt.Errorf("resolve base image %s: %w", stage.Base, err)
	}

	key := "base:" + stage.Base
	var chain []layer
	if ref, err := image.ParseRef(stage.Base); err == nil {
		if digest := image.BaseDigest(ex.stateRoot, ref.Name()); digest != "" {
			key = "base:" + digest
		}
		chain = ex.layersFromRecord(ref.Name())
	}
	if len(chain) == 0 {
		// No usable record: fall back to the merged rootfs as a single
		// layer. Correct, just heavier.
		rootfs, err := image.MergedRootfs(ex.stateRoot, stage.Base)
		if err != nil {
			return fmt.Errorf("resolve base rootfs: %w", err)
		}
		chain = []layer{{Path: rootfs, Key: key, Index: -1}}
	}

	if !ex.layered {
		if err := ex.preparePlainRoot(chain); err != nil {
			return err
		}
		// The fallback mutates one copy, so the whole thing is the diff and
		// counts as a single build layer for push purposes.
		ex.baseCount = 0
		return nil
	}

	ex.chain = chain
	ex.baseCount = len(chain)
	return nil
}

// closeStage records a finished stage: its merged root becomes available to
// `copy --from=`, and its layer chain becomes available to a later
// `base <stage>`.
func (ex *executor) closeStage(stage config.Stage, last bool) error {
	if stage.Name == "" {
		return nil
	}

	if ex.layered {
		if len(ex.chain) <= ex.baseCount {
			return nil // a stage that changed nothing has nothing to export
		}
		lowers := make([]string, 0, len(ex.chain))
		for _, l := range ex.chain {
			lowers = append(lowers, l.Path)
		}
		rootfs, err := overlayfs.Merge(ex.stateRoot, "stage-"+sanitizeTag(stage.Name), lowers)
		if err != nil {
			return fmt.Errorf("assemble stage %q: %w", stage.Name, err)
		}
		ex.stageRoots[stage.Name] = rootfs
	} else if !last && ex.plainRoot != "" {
		// Without overlayfs the stage root is the writable copy, which the
		// next stage will overwrite. Snapshot it so `copy --from=` still
		// reads the right filesystem.
		stageDir := filepath.Join(ex.stateRoot, "build-work", "stage-"+sanitizeTag(stage.Name))
		if err := os.RemoveAll(stageDir); err != nil {
			return err
		}
		if err := overlayfs.CopyTree(ex.plainRoot, stageDir); err != nil {
			return fmt.Errorf("snapshot stage %q: %w", stage.Name, err)
		}
		ex.stageRoots[stage.Name] = stageDir
	}

	if !last {
		ex.stageChains[stage.Name] = append([]layer(nil), ex.chain...)
	}
	return nil
}

func (ex *executor) layersFromRecord(refName string) []layer {
	dirs := image.LayerDirs(ex.stateRoot, refName)
	if len(dirs) == 0 {
		return nil
	}
	chain := make([]layer, 0, len(dirs))
	for _, dir := range dirs {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return nil
		}
		chain = append(chain, layer{Path: dir, Key: filepath.Base(dir), Index: -1})
	}
	return chain
}

func (ex *executor) preparePlainRoot(chain []layer) error {
	plain := filepath.Join(ex.stateRoot, "build-work", "plain-root")
	if err := os.RemoveAll(plain); err != nil {
		return err
	}
	if err := overlayfs.CopyTree(chain[0].Path, plain); err != nil {
		return fmt.Errorf("prepare build root: %w", err)
	}
	for _, l := range chain[1:] {
		if err := overlayfs.CopyTree(l.Path, plain); err != nil {
			return fmt.Errorf("prepare build root: %w", err)
		}
	}
	ex.plainRoot = plain
	return nil
}

func (ex *executor) run(stage config.Stage) error {
	g := NewGraph(stage.Steps)

	// Parallel execution is only safe with per-step layers: without them
	// every step would mutate one shared rootfs copy and race. With layers,
	// each concurrent step gets its own mountpoint, its own upperdir and its
	// own state snapshot.
	workers := 1
	if ex.layered && ex.opts.Workers > 1 {
		workers = ex.opts.Workers
	}
	if workers > 1 {
		fmt.Fprintf(ex.out, "executing %d steps, %d workers\n", g.Len(), workers)
	} else {
		fmt.Fprintf(ex.out, "executing %d steps sequentially\n", g.Len())
	}

	return g.RunConcurrent(workers, func(n Node) error {
		return ex.step(n)
	})
}

// step executes one build step, consulting the cache first.
func (ex *executor) step(n Node) error {
	step := n.Step
	started := time.Now()
	res := StepResult{Index: n.Index, Directive: step.Type}

	// Metadata-only steps never touch the filesystem: no layer, no cache.
	// Their effect is already baked into the snapshots the graph computed,
	// so there is no shared state to update here.
	switch step.Type {
	case config.DirectiveWorkdir, config.DirectiveEnv,
		config.DirectiveExpose, config.DirectiveStart:
		return ex.record(n, res, started)
	}

	// Copy steps are content-addressed automatically: their key is the
	// hash of the files they read. exec steps are NOT auto-cached — a
	// command's result depends on the world, not just its argv — so they
	// only participate when the Sentrafile supplies an explicit
	// `cache key=` override.
	var key string
	if step.Type == config.DirectiveCopy {
		// A --from copy reads the earlier stage's filesystem, not the
		// build context, so that is what gets hashed.
		sourceRoot := ex.opts.ContextDir
		if step.From != "" {
			root, err := ex.stageRoot(step.From)
			if err != nil {
				return err
			}
			sourceRoot = root
		}
		inputs, err := HashInputs(ex.cache, sourceRoot, step.Src, ex.opts.Workers)
		if err != nil {
			return err
		}
		key = AutoKey(step, ex.parentKey(), inputs)
		res.CacheKey = key
	} else if step.Cache != "" {
		key = ManualKey(step.Cache)
		res.CacheKey = key
		res.ManualKey = true
	}

	if key != "" {
		if entry, ok := ex.cache.Lookup(key); ok {
			res.CacheHit = true
			ex.pushLayer(layer{Path: entry.Layer, Key: key, Index: n.Index})
			fmt.Fprintf(ex.out, "[%d] %-7s cache hit  %s\n", n.Index, step.Type, step.Summary())
			return ex.record(n, res, started)
		}
	}

	if err := ex.execute(n, step, key); err != nil {
		return err
	}
	return ex.record(n, res, started)
}

// execute runs a mutating step against a fresh upperdir and caches the
// resulting diff.
func (ex *executor) execute(n Node, step config.Step, key string) error {
	work := filepath.Join(ex.stateRoot, "build-work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		return err
	}

	root, unmount, err := ex.mountStep(n, step)
	if err != nil {
		return err
	}

	fmt.Fprintf(ex.out, "[%d] %-7s %s\n", n.Index, step.Type, step.Summary())

	// A Sentrafile workdir must exist in the build root before anything
	// resolves a destination or a cwd against it. Docker's WORKDIR creates
	// the directory; we do the same, per step, so the creation is captured
	// in this step's layer. The workdir comes from this step's snapshot, so
	// a concurrent WORKDIR cannot move it.
	if n.State.Workdir != "" {
		dir := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(n.State.Workdir, "/")))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			unmount()
			return fmt.Errorf("create workdir %q: %w", n.State.Workdir, err)
		}
	}

	var stepErr error
	switch step.Type {
	case config.DirectiveCopy:
		stepErr = ex.runCopy(step, root, n.State.Workdir)
	case config.DirectiveExec:
		stepErr = ex.runExec(n, step, root)
	default:
		stepErr = fmt.Errorf("cannot execute directive %q", step.Type)
	}

	// The overlay must be torn down before its upperdir can be moved into
	// the cache.
	unmount()
	if stepErr != nil {
		return stepErr
	}

	// The mounted upperdir is this step's filesystem diff. Every successful
	// layered mutating step must become part of the next step's lower stack,
	// including ordinary (uncacheable) exec steps and --no-cache builds.
	if ex.layered {
		if key != "" && ex.cache.enabled {
			entry, err := ex.cache.Store(key, ex.upperDir(n))
			if err != nil {
				return fmt.Errorf("cache step %d: %w", n.Index, err)
			}
			ex.pushLayer(layer{Path: entry.Layer, Key: key, Index: n.Index})
		} else {
			ex.pushLayer(layer{
				Path:  ex.upperDir(n),
				Key:   fmt.Sprintf("uncached:%s:%d", ex.buildID, n.Index),
				Index: n.Index,
			})
		}
	}
	return nil
}

func (ex *executor) runCopy(step config.Step, root, workdir string) error {
	dest := step.Dest
	if workdir != "" && !filepath.IsAbs(filepath.FromSlash(dest)) {
		dest = strings.TrimSuffix(workdir, "/") + "/" + dest
	}
	sourceRoot := ex.opts.ContextDir
	if step.From != "" {
		stageRoot, err := ex.stageRoot(step.From)
		if err != nil {
			return err
		}
		sourceRoot = stageRoot
	}
	return copyInto(sourceRoot, root, step.Src, dest)
}

// stageRoot returns the merged rootfs of a named stage, which is what
// `copy --from=<name>` reads.
func (ex *executor) stageRoot(name string) (string, error) {
	ex.mu.Lock()
	root, ok := ex.stageRoots[name]
	ex.mu.Unlock()
	if !ok {
		return "", fmt.Errorf("stage %q has nothing to copy from (it produced no changes)", name)
	}
	if _, err := os.Stat(root); err != nil {
		return "", fmt.Errorf("stage %q rootfs is unavailable: %w", name, err)
	}
	return root, nil
}

func (ex *executor) upperDir(n Node) string {
	return filepath.Join(ex.stateRoot, "build-work", fmt.Sprintf("upper-%d", n.Index))
}

// mountStep exposes the current chain plus a fresh upperdir as one
// writable root, and returns a teardown function.
func (ex *executor) mountStep(n Node, step config.Step) (string, func(), error) {
	if !ex.layered {
		return ex.plainRoot, func() {}, nil
	}

	ex.mu.Lock()
	lowers := make([]string, 0, len(ex.chain))
	for _, l := range ex.chain {
		lowers = append(lowers, l.Path)
	}
	ex.mu.Unlock()

	upper := ex.upperDir(n)
	if err := os.RemoveAll(upper); err != nil {
		return "", func() {}, err
	}
	if err := os.MkdirAll(upper, 0o755); err != nil {
		return "", func() {}, err
	}

	// Each step gets its own mountpoint, matching its own upperdir. A
	// shared mountpoint here would race under concurrent workers: one
	// step's unmount (torn down after it finishes) or remount (started by
	// another worker) can pull the rug out from under a step that is mid
	// copy/exec, leaving that step looking at an empty directory.
	mountpoint := filepath.Join(ex.stateRoot, "build-work", fmt.Sprintf("root-%d", n.Index))
	if err := os.MkdirAll(mountpoint, 0o755); err != nil {
		return "", func() {}, err
	}
	work := upper + "-work"
	if err := os.MkdirAll(work, 0o755); err != nil {
		return "", func() {}, err
	}

	unmount, err := overlayfs.MountStep(lowers, upper, work, mountpoint)
	if err != nil {
		return "", func() {}, fmt.Errorf("mount step root: %w", err)
	}
	return mountpoint, func() { _ = unmount() }, nil
}

// resolveCommand locates a step's argv[0] inside the build root the same
// way the container runtime will, so a missing binary is reported with the
// paths that were actually searched instead of a failure from deep inside
// runc.
func resolveCommand(root, argv0 string, env []string) error {
	if strings.ContainsRune(argv0, filepath.Separator) {
		candidate := filepath.Join(root, strings.TrimPrefix(argv0, "/"))
		if _, err := os.Lstat(candidate); err != nil {
			return fmt.Errorf("command %q not found in build root: %s", argv0, candidate)
		}
		return nil
	}

	searchPath := ""
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, "PATH="); ok {
			searchPath = v
		}
	}
	if searchPath == "" {
		searchPath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	}

	for _, dir := range strings.Split(searchPath, ":") {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(root, strings.TrimPrefix(dir, "/"), argv0)
		// Do not use Stat here: image commands commonly are absolute
		// symlinks (Alpine's /bin/sh -> /bin/busybox). Stat would resolve
		// that link against the host rather than the container root and
		// incorrectly claim the command is missing. runc resolves it after
		// switching to the container rootfs.
		if info, err := os.Lstat(candidate); err == nil && !info.IsDir() {
			return nil
		}
	}
	return fmt.Errorf("command %q not found in build root %s (searched PATH=%s)", argv0, root, searchPath)
}

// runExec runs a Sentrafile exec step inside the build root through
// runc — the same runtime path `sentra run` uses.
func (ex *executor) runExec(n Node, step config.Step, root string) error {
	env := append([]string(nil), n.State.Env...)
	workdir := n.State.Workdir

	if len(env) == 0 {
		env = []string{
			"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			"HOME=/root",
			"TERM=xterm",
		}
	}
	cwd := workdir
	if cwd == "" {
		cwd = "/"
	}

	id, err := rt.NewID()
	if err != nil {
		return err
	}
	if err := resolveCommand(root, step.Argv[0], env); err != nil {
		return err
	}
	c := &rt.Container{
		ID: fmt.Sprintf("build-%d-%s", n.Index, id),
		// A build step must be able to write; the read-only rootfs default
		// is a runtime concern, not a build-time one.
		Rootfs:   root,
		Cmd:      step.Argv,
		Readonly: false,
		Env:      env,
		Cwd:      cwd,
	}
	if err := rt.CreateBundle(c, rt.DefaultLimits()); err != nil {
		return err
	}
	defer os.RemoveAll(c.Bundle)

	r := rt.NewRunc()
	code, runErr := r.Run(c.ID, c.Bundle, ex.out, ex.out)
	if code != 0 {
		return fmt.Errorf("exec step %d exited %d: %s", n.Index, code, strings.Join(step.Argv, " "))
	}
	if runErr != nil {
		return fmt.Errorf("exec step %d: %w", n.Index, runErr)
	}
	return nil
}

// assemble produces the final merged rootfs for the built image.
// assembleImage merges the final stage into the runnable rootfs the build
// record points at. A multi-stage build's image contains only the final
// stage's base plus the layers that stage added: earlier stages contribute
// through `copy --from=`, exactly as a user would expect, so a compiler
// toolchain never ships inside the runtime image.
func (ex *executor) assembleImage() (*Image, error) {
	ex.mu.Lock()
	lowers := make([]string, 0, len(ex.chain))
	if !ex.layered && ex.plainRoot != "" {
		lowers = append(lowers, ex.plainRoot)
	}
	for _, l := range ex.chain {
		lowers = append(lowers, l.Path)
	}
	ex.mu.Unlock()

	tag := ex.opts.Tag
	if tag == "" {
		tag = "latest"
	}
	name := "build-" + sanitizeTag(tag)

	var rootfs string
	var err error

	if !ex.layered {
		// The fallback mutates one scratch directory that the next build
		// reuses, so the finished image gets its own stable copy. Without
		// this, every tag would point at the most recent build's rootfs.
		rootfs = filepath.Join(ex.stateRoot, "built", sanitizeTag(tag)+"-rootfs")
		if err := os.RemoveAll(rootfs); err != nil {
			return nil, err
		}
		if err := overlayfs.CopyTree(ex.plainRoot, rootfs); err != nil {
			return nil, fmt.Errorf("materialize image rootfs: %w", err)
		}
	} else if len(lowers) == 1 {
		// Nothing to stack: the single root is already the image.
		rootfs = lowers[0]
	} else {
		rootfs, err = overlayfs.Merge(ex.stateRoot, name, lowers)
		if err != nil {
			return nil, fmt.Errorf("assemble image rootfs: %w", err)
		}
	}

	// Without overlayfs there is no per-step diff to upload, only the whole
	// filesystem, so a fallback build is deliberately not pushable.
	buildLayers := []string{}
	if ex.layered && len(lowers) > ex.baseCount {
		buildLayers = lowers[ex.baseCount:]
	}

	return &Image{
		Tag:         tag,
		Base:        ex.finalStage.Base,
		Rootfs:      rootfs,
		Env:         ex.final.Env,
		Expose:      ex.final.Expose,
		Entrypoint:  ex.final.Entrypoint,
		Security:    ex.plan.Security,
		Layers:      lowers,
		BuildLayers: buildLayers,
		Fallback:    !ex.layered,
		Steps:       ex.orderedResults(),
		BuiltAt:     time.Now(),
	}, nil
}

func (ex *executor) writeImageRecord(img *Image) error {
	dir := filepath.Join(ex.stateRoot, "built")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(img, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, sanitizeTag(img.Tag)+".json"), data, 0o644)
}

func (ex *executor) record(n Node, res StepResult, started time.Time) error {
	res.Duration = time.Since(started)
	res.Index = n.Index + ex.stepBase
	ex.mu.Lock()
	ex.results[res.Index] = res
	ex.ranSteps++
	ex.mu.Unlock()
	return nil
}

func (ex *executor) orderedResults() []StepResult {
	out := make([]StepResult, 0, ex.ranSteps)
	for i := 0; i < ex.ranSteps; i++ {
		if r, ok := ex.results[i]; ok {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out
}

func (ex *executor) parentKey() string {
	ex.mu.Lock()
	defer ex.mu.Unlock()
	if len(ex.chain) == 0 {
		return "empty"
	}
	return ex.chain[len(ex.chain)-1].Key
}

// pushLayer inserts a layer into the chain at its Sentrafile position.
// Workers finish out of order, so appending would silently reverse layers
// and make the next step's cache key depend on scheduling.
func (ex *executor) pushLayer(l layer) {
	ex.mu.Lock()
	defer ex.mu.Unlock()

	at := len(ex.chain)
	for i, existing := range ex.chain {
		if l.Index < existing.Index {
			at = i
			break
		}
	}
	ex.chain = append(ex.chain, layer{})
	copy(ex.chain[at+1:], ex.chain[at:])
	ex.chain[at] = l
}

// mergeEnv applies Sentrafile env pairs, later assignments winning.
func mergeEnv(existing, pairs []string) []string {
	out := append([]string(nil), existing...)
	for _, pair := range pairs {
		key, _, _ := strings.Cut(pair, "=")
		replaced := false
		for i, e := range out {
			if k, _, _ := strings.Cut(e, "="); k == key {
				out[i] = pair
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, pair)
		}
	}
	return out
}

func sanitizeTag(tag string) string {
	var b strings.Builder
	for _, r := range tag {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		return "latest"
	}
	return b.String()
}
