import React, { useEffect, useRef, useState } from "react";
import { CONFIG } from "../config";
import { useReveal } from "../hooks";

function fmtVal(v, unit) {
  return v > 0 ? v + unit : "TBD";
}

function Bar({ label, value, unit, max, dim }) {
  const [width, setWidth] = useState(0);
  const [text, setText] = useState(fmtVal(0, unit));
  const started = useRef(false);

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    const calm = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    setWidth((value / Math.max(max, 0.0001)) * 100);
    if (value <= 0 || calm) {
      setText(fmtVal(value, unit));
      return;
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

export default function Benchmarks() {
  const ref = useReveal();
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

  const { startup, memory } = CONFIG.benchmarks;

  return (
    <section id="benchmarks" aria-label="Benchmarks" ref={ref}>
      <h2 className="section-title reveal">Small numbers, honestly measured</h2>
      <p className="section-sub reveal">
        <span className="mono">sentra run</span> vs <span className="mono">docker run</span>, same image.
      </p>
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
      <p className="bench-note reveal">Results pending the benchmark suite — values above are placeholders.</p>
    </section>
  );
}
