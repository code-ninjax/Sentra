import React, { useEffect, useRef, useState } from "react";
import { CONFIG } from "../config";
import { useReveal } from "../hooks";

function fmtVal(v, unit) {
  return v > 0 ? v + unit : "TBD";
}

function Bar({ label, value, unit, max, dim }) {
  const [width, setWidth] = useState(0);
  const [text, setText] = useState(fmtVal(0, unit));

  // No started-guard: StrictMode remounts must reschedule, cleanup cancels.
  useEffect(() => {
    const calm = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    setWidth((value / Math.max(max, 0.0001)) * 100);
    if (value <= 0 || calm) {
      setText(fmtVal(value, unit));
      return undefined;
    }
    const t0 = performance.now();
    const dur = 1200;
    let raf = 0;
    const tick = (t) => {
      const k = Math.min(1, (t - t0) / dur);
      const eased = 1 - Math.pow(1 - k, 3);
      setText((value * eased).toFixed(value < 10 ? 2 : 0) + unit);
      if (k < 1) raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [value, unit, max]);

  return (
    <div className="bench-row">
      <div className="lbl">
        <span>{label}</span>
        <span className="val">{text}</span>
      </div>
      <div className={"bench-bar" + (dim ? " dim" : "")}>
        <i style={{ width: width + "%" }} />
      </div>
    </div>
  );
}

// Bar order matches benchScene cluster build order:
// [startup·sentra, startup·docker, memory·sentra, memory·docker].
function buildBars() {
  const { startup, memory } = CONFIG.benchmarks;
  return [
    { metric: "Startup time", runtime: "sentra", value: startup.sentra, unit: startup.unit, n: startup.n, stamp: startup.stamp },
    { metric: "Startup time", runtime: "docker", value: startup.docker, unit: startup.unit, n: startup.n, stamp: startup.stamp },
    { metric: "Memory footprint", runtime: "sentra", value: memory.sentra, unit: memory.unit, n: memory.n, stamp: memory.stamp },
    { metric: "Memory footprint", runtime: "docker", value: memory.docker, unit: memory.unit, n: memory.n, stamp: memory.stamp },
  ];
}

const METHOD =
  "Wall-clock of a full CLI run of /bin/true (startup) and VmHWM of a detached sleep init (memory). Same pinned alpine image. Raw samples in benchmarks/results/.";

// Static config — build once at module level.
const BARS = buildBars();

export default function Benchmarks({ registerTheme }) {
  const ref = useReveal();
  const canvas = useRef(null);
  const api = useRef(null);
  const [sel, setSel] = useState(0);

  useEffect(() => {
    let alive = true;
    let sceneApi = null;
    const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    const isMobile = window.matchMedia("(max-width: 900px)").matches;

    const boot = () => {
      if (!alive) return;
      // Dynamic import: three.js loads only when the section nears view.
      import("../three/benchScene.js")
        .then(({ initBenchScene }) =>
          initBenchScene({
            canvas: canvas.current,
            bars: BARS,
            reduceMotion,
            isMobile,
            onHover: () => {},
            onSelect: (i) => setSel(i),
          })
        )
        .then((a) => {
          if (!alive) {
            a.dispose();
            return;
          }
          sceneApi = a;
          api.current = a;
          if (registerTheme) registerTheme((t) => a.retheme(t));
          a.select(0);
        })
        .catch(() => {
          /* 2D bars below remain the readable fallback */
          if (canvas.current) canvas.current.style.display = "none";
        });
    };

    const target = canvas.current ? canvas.current.parentElement : null;
    const obs = new IntersectionObserver(
      (entries) => {
        entries.forEach((e) => {
          if (e.isIntersecting) {
            boot();
            obs.disconnect();
          }
        });
      },
      { rootMargin: "200px" }
    );
    if (target) obs.observe(target);
    return () => {
      alive = false;
      obs.disconnect();
      if (sceneApi) sceneApi.dispose();
    };
  }, [BARS, registerTheme]);

  useEffect(() => {
    const fine = window.matchMedia("(pointer: fine)").matches;
    const calm = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    if (!fine || calm) return undefined;
    const onLean = (e) => {
      if (api.current) api.current.lean(e.detail.x, e.detail.y);
    };
    window.addEventListener("sentra-lean", onLean);
    return () => window.removeEventListener("sentra-lean", onLean);
  }, []);

  const pick = (i) => {
    setSel(i);
    if (api.current) api.current.select(i);
  };

  const s = BARS[sel];
  const tbd = s.value <= 0;

  const { startup, memory } = CONFIG.benchmarks;
  const [visible, setVisible] = useState(false);
  const section = useRef(null);

  useEffect(() => {
    const el = section.current;
    if (!el) return;
    const obs = new IntersectionObserver(
      (entries) => {
        entries.forEach((e) => {
          if (e.isIntersecting) {
            setVisible(true);
            obs.disconnect();
          }
        });
      },
      { threshold: 0.3 }
    );
    obs.observe(el);
    return () => obs.disconnect();
  }, []);

  return (
    <section id="benchmarks" aria-label="Benchmarks" ref={ref}>
      <h2 className="section-title reveal">Small numbers, honestly measured</h2>
      <p className="section-sub reveal">
        <span className="mono">sentra run</span> vs <span className="mono">docker run</span>, same image.
        Drag the chart. Hover or tap a bar.
      </p>
      <div className="bench-3d-layout">
        <div className="bench-stage reveal">
          <canvas
            id="bench-canvas"
            ref={canvas}
            tabIndex={0}
            aria-label="Interactive 3D benchmark chart. Use left and right arrow keys to inspect each bar."
            onKeyDown={(e) => {
              if (e.key === "ArrowRight") pick((sel + 1) % BARS.length);
              if (e.key === "ArrowLeft") pick((sel - 1 + BARS.length) % BARS.length);
            }}
          />
        </div>
        <div className="feature-side reveal">
          <div className="bench-detail">
            <h3>
              <span className="idx">
                {s.metric} · {s.runtime}
              </span>
              <span>{tbd ? "TBD" : `${s.value}${s.unit}`}</span>
            </h3>
            <p>
              {tbd
                ? `No docker number yet — enable the Docker Desktop WSL integration and rerun scripts/benchmark.sh. Ghost bar, not a guess.`
                : `Median over ${s.n} runs, ${s.stamp}, same pinned image both sides.`}
            </p>
            <div className="feature-stat mono">{METHOD}</div>
          </div>
          <div className="bench-legend" role="tablist" aria-label="Chart bars">
            {BARS.map((b, i) => (
              <button
                key={i}
                role="tab"
                aria-selected={i === sel}
                className={"bench-chip" + (i === sel ? " active" : "")}
                onClick={() => pick(i)}
              >
                {b.metric} · {b.runtime}
              </button>
            ))}
          </div>
        </div>
      </div>
      <div className="bench-grid" ref={section}>
        <div className="bench-card reveal">
          <h3>Startup time</h3>
          {visible && (
            <>
              <Bar label="sentra" value={startup.sentra} unit={startup.unit} max={Math.max(startup.sentra, startup.docker)} />
              <Bar label="docker" value={startup.docker} unit={startup.unit} max={Math.max(startup.sentra, startup.docker)} dim />
            </>
          )}
        </div>
        <div className="bench-card reveal">
          <h3>Memory footprint</h3>
          {visible && (
            <>
              <Bar label="sentra" value={memory.sentra} unit={memory.unit} max={Math.max(memory.sentra, memory.docker)} />
              <Bar label="docker" value={memory.docker} unit={memory.unit} max={Math.max(memory.sentra, memory.docker)} dim />
            </>
          )}
        </div>
      </div>
      <p className="bench-note reveal">Median of 20 startup / 5 memory runs, alpine:3.20, WSL2 4CPU. Raw samples in benchmarks/results/.</p>
    </section>
  );
}
