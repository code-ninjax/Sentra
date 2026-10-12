import React, { useEffect, useRef, useState } from "react";
import { STACKS } from "../config";
import { deviconFor } from "../devicons";
import { useReveal } from "../hooks";

export default function StackSection({ registerTheme }) {
  const ref = useReveal();
  const canvas = useRef(null);
  const api = useRef(null);
  const [active, setActive] = useState(4);

  useEffect(() => {
    let alive = true;
    let sceneApi = null;
    const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    const isMobile = window.matchMedia("(max-width: 900px)").matches;

    const boot = () => {
      if (!alive) return;
      // Dynamic import: three.js loads only when the section nears view.
      import("../three/stackScene.js")
        .then(({ initStackScene }) =>
          initStackScene({
            canvas: canvas.current,
            items: STACKS.map((it) => ({ ...it, iconUrl: deviconFor(it.name) })),
            reduceMotion,
            isMobile,
            onSelect: (i) => setActive(i),
          })
        )
        .then((a) => {
          if (!alive) {
            a.dispose();
            return;
          }
          sceneApi = a;
          api.current = a;
          registerTheme((t) => a.retheme(t));
          a.select(4);
        })
        .catch(() => {
          /* static stage remains; chips still work */
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
  }, [registerTheme]);

  const pick = (i) => {
    setActive(i);
    if (api.current) api.current.select(i);
  };

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

  const s = STACKS[active];

  return (
    <section id="stack" aria-label="Works with your stack" ref={ref}>
      <h2 className="section-title reveal">Works with your stack</h2>
      <p className="section-sub reveal">Any OCI base runs unmodified. Hover a tile, click to inspect.</p>
      <div className="stack-layout">
        <div className="stack-stage reveal">
          <canvas
            id="stack-canvas"
            ref={canvas}
            tabIndex={0}
            aria-label="Interactive 3D grid of supported stacks. Use left and right arrow keys to switch."
            onKeyDown={(e) => {
              if (e.key === "ArrowRight") pick((active + 1) % STACKS.length);
              if (e.key === "ArrowLeft") pick((active - 1 + STACKS.length) % STACKS.length);
            }}
          />
        </div>
        <div className="feature-side reveal">
          <div className="stack-detail">
            <h3>
              <img className="stack-logo" src={deviconFor(s.name)} alt="" width="36" height="36" />
              {s.name}
            </h3>
            <p>{s.blurb}</p>
            <div className="feature-stat mono">
              sentra pull {s.name.toLowerCase().replace(/[^a-z0-9]+/g, "")}:latest
            </div>
          </div>
          <div className="stack-grid" role="tablist" aria-label="Stacks">
            {STACKS.map((item, i) => (
              <button
                key={item.name}
                role="tab"
                aria-selected={i === active}
                className={"stack-chip" + (i === active ? " active" : "")}
                onClick={() => pick(i)}
              >
                <img src={deviconFor(item.name)} alt="" width="28" height="28" loading="lazy" />
                <span>{item.name}</span>
              </button>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}
