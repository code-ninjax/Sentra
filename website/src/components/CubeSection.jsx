import React, { useEffect, useRef, useState } from "react";
import { FACES } from "../config";
import { Icon } from "../icons";
import { useReveal } from "../hooks";

export default function CubeSection({ registerTheme }) {
  const ref = useReveal();
  const canvas = useRef(null);
  const placeholder = useRef(null);
  const fallback = useRef(null);
  const wires = useRef(null);
  const stage = useRef(null);
  const api = useRef(null);
  const [active, setActive] = useState(0);

  // chips live in React; the scene reads them for wire targets
  const chipRefs = useRef([]);

  useEffect(() => {
    let alive = true;
    let sceneApi = null;
    const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    const isMobile = window.matchMedia("(max-width: 900px)").matches;

    const boot = () => {
      if (!alive) return;
      // Dynamic import: three.js loads only when the section nears view.
      import("../three/cubeScene.js")
        .then(({ initCubeScene }) =>
          initCubeScene({
            canvas: canvas.current,
            placeholder: placeholder.current,
            fallback: fallback.current,
            wiresEl: wires.current,
            faces: FACES,
            chips: chipRefs.current,
            reduceMotion,
            isMobile,
            onHover: () => {},
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
          a.setActive(0, false);
        })
        .catch(() => {
          if (!alive) return;
          canvas.current.style.display = "none";
          placeholder.current.classList.add("hidden");
          fallback.current.classList.add("show");
          wires.current.style.display = "none";
        });
    };

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
    if (stage.current) obs.observe(stage.current);
    return () => {
      alive = false;
      obs.disconnect();
      if (sceneApi) sceneApi.dispose();
    };
  }, [registerTheme]);

  const setFace = (i, lock) => {
    const n = ((i % FACES.length) + FACES.length) % FACES.length;
    setActive(n);
    if (api.current) api.current.setActive(n, lock);
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

  const f = FACES[active];

  return (
    <section id="features" aria-label="Features" ref={ref}>
      <h2 className="section-title reveal">Six ideas, one binary</h2>
      <p className="section-sub reveal">Drag the cube. Click a face to lock it.</p>
      <div className="cube-layout" id="cube-layout">
        <div className="cube-stage reveal" ref={stage}>
          <canvas
            id="cube-canvas"
            ref={canvas}
            tabIndex={0}
            aria-label="Interactive 3D cube showing Sentra features. Use left and right arrow keys to switch faces."
            onKeyDown={(e) => {
              if (e.key === "ArrowRight") setFace(active + 1, true);
              if (e.key === "ArrowLeft") setFace(active - 1, true);
            }}
          />
          <div className="cube-placeholder" ref={placeholder}>
            loading 3D…
          </div>
          <div className="css-cube-fallback" ref={fallback} aria-hidden="true">
            <div className="css-cube">
              <div></div>
              <div></div>
              <div></div>
              <div></div>
              <div></div>
              <div></div>
            </div>
          </div>
        </div>
        <div className="feature-side reveal">
          <div className="feature-detail">
            <h3>
              <span className="idx">
                {("0" + (active + 1)).slice(-2)} / 06
              </span>
              <span>{f.title}</span>
            </h3>
            <p>{f.desc}</p>
            <div className="feature-stat mono">{f.stat}</div>
          </div>
          <div className="face-chips" role="tablist" aria-label="Feature faces">
            {FACES.map((face, i) => (
              <button
                key={face.title}
                ref={(el) => {
                  chipRefs.current[i] = el;
                }}
                role="tab"
                aria-selected={i === active}
                className={"face-chip" + (i === active ? " active" : "")}
                onClick={() => setFace(i, true)}
              >
                {face.title}
              </button>
            ))}
          </div>
          <div className="face-nav">
            <button onClick={() => setFace(active - 1, true)} aria-label="Previous feature">
              <Icon name="chevL" size={16} />
            </button>
            <button onClick={() => setFace(active + 1, true)} aria-label="Next feature">
              <Icon name="chevR" size={16} />
            </button>
          </div>
        </div>
        <svg id="cube-wires" ref={wires} aria-hidden="true"></svg>
      </div>
      <div className="feature-cards">
        {FACES.map((face) => (
          <div className="f-card reveal" key={face.title}>
            <div className="f-icon">
              <Icon name={face.icon} size={26} />
            </div>
            <h3>{face.title}</h3>
            <p>{face.desc}</p>
            <code>{face.stat}</code>
          </div>
        ))}
      </div>
    </section>
  );
}
