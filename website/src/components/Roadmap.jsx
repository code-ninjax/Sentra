import React, { useEffect, useRef } from "react";
import { ROADMAP } from "../config";
import { Icon } from "../icons";
import { useReveal } from "../hooks";

// Scattered constellation layout on desktop: node cards at varied offsets
// and tilts, linked by animated dashed SVG lines. Vertical stack on mobile.
const SLOTS = [
  { left: "2%", top: "6%", tilt: "-2deg" },
  { left: "52%", top: "0%", tilt: "1.5deg" },
  { left: "28%", top: "38%", tilt: "2deg" },
  { left: "68%", top: "52%", tilt: "-1.5deg" },
  { left: "4%", top: "64%", tilt: "-1deg" },
];
// Links as index pairs into ROADMAP.
const LINKS = [
  [0, 2],
  [2, 1],
  [2, 3],
  [1, 3],
  [0, 4],
  [4, 2],
];

export default function Roadmap() {
  const ref = useReveal();
  const stage = useRef(null);
  const svg = useRef(null);

  useEffect(() => {
    const el = stage.current;
    const svgEl = svg.current;
    if (!el || !svgEl) return;
    const mobile = () => window.matchMedia("(max-width: 900px)").matches;

    const draw = () => {
      if (mobile()) {
        svgEl.innerHTML = "";
        return;
      }
      const sr = el.getBoundingClientRect();
      const nodes = Array.from(el.querySelectorAll(".road-node"));
      if (nodes.length !== ROADMAP.length) return;
      svgEl.setAttribute("viewBox", `0 0 ${sr.width} ${sr.height}`);
      const center = (n) => {
        const r = n.getBoundingClientRect();
        return { x: r.left - sr.left + r.width / 2, y: r.top - sr.top + r.height / 2 };
      };
      const pts = nodes.map(center);
      svgEl.innerHTML = LINKS.map(([a, b]) => {
        const p = pts[a],
          q = pts[b];
        const mx = (p.x + q.x) / 2;
        const len = Math.hypot(q.x - p.x, q.y - p.y);
        return (
          `<path d="M ${p.x} ${p.y} C ${mx} ${p.y}, ${mx} ${q.y}, ${q.x} ${q.y}" ` +
          `fill="none" stroke="currentColor" stroke-width="1.5" ` +
          `style="color:var(--accent);stroke-dasharray:7 7;opacity:0.65" data-len="${len}"/>`
        );
      }).join("");
    };

    // position nodes, then draw after layout settles
    const t = setTimeout(draw, 60);
    const ro = new ResizeObserver(draw);
    ro.observe(el);
    window.addEventListener("resize", draw);
    return () => {
      clearTimeout(t);
      ro.disconnect();
      window.removeEventListener("resize", draw);
    };
  }, []);

  return (
    <section id="roadmap" aria-label="Roadmap" ref={ref}>
      <h2 className="section-title reveal">Coming soon</h2>
      <p className="section-sub reveal">Not done. Next on the list — explicitly post-MVP, not promises.</p>
      <div className="roadmap-stage reveal" ref={stage}>
        <svg id="roadmap-wires" ref={svg} aria-hidden="true"></svg>
        {ROADMAP.map((r, i) => (
          <div
            key={r.title}
            className="road-node"
            style={{ left: SLOTS[i].left, top: SLOTS[i].top, ["--tilt"]: SLOTS[i].tilt }}
          >
            <div className="road-icon">
              <Icon name={r.icon} size={26} />
            </div>
            <h3>{r.title}</h3>
            <p>{r.desc}</p>
            <span className="pill">Planned</span>
          </div>
        ))}
      </div>
    </section>
  );
}
