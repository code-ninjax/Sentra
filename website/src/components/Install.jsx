import React, { useEffect, useRef, useState } from "react";
import { CONFIG } from "../config";
import { Icon } from "../icons";
import { useReveal } from "../hooks";

const TYPE_CMD = "sentra run -t demo";
const TYPE_OUT = ["sentra: rootless, read-only rootfs, seccomp=default", "hello from a sentra build"];

export default function Install() {
  const ref = useReveal();
  const [typed, setTyped] = useState("");
  const [lines, setLines] = useState([]);
  const [copyLabel, setCopyLabel] = useState("Copy");
  const started = useRef(false);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const obs = new IntersectionObserver(
      (entries) => {
        entries.forEach((e) => {
          if (!e.isIntersecting || started.current) return;
          started.current = true;
          const calm = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
          if (calm) {
            setTyped(TYPE_CMD);
            setLines(TYPE_OUT);
            return;
          }
          let i = 0;
          const iv = setInterval(() => {
            i += 1;
            setTyped(TYPE_CMD.slice(0, i));
            if (i >= TYPE_CMD.length) {
              clearInterval(iv);
              let l = 0;
              const iv2 = setInterval(() => {
                l += 1;
                setLines(TYPE_OUT.slice(0, l));
                if (l >= TYPE_OUT.length) clearInterval(iv2);
              }, 450);
            }
          }, 70);
        });
      },
      { threshold: 0.4 }
    );
    obs.observe(el);
    return () => obs.disconnect();
  }, []);

  const onCopy = () => {
    const done = (ok) => {
      setCopyLabel(ok ? "Copied" : "Failed");
      setTimeout(() => setCopyLabel("Copy"), 1600);
    };
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(CONFIG.INSTALL_CMD).then(() => done(true), () => done(false));
    } else {
      done(false);
    }
  };

  return (
    <section id="install" aria-label="Install" ref={ref}>
      <h2 className="section-title reveal">Get started in seconds</h2>
      <p className="section-sub reveal">
        One binary. No daemon. No VM.{" "}
        <a href={CONFIG.RELEASES_URL} target="_blank" rel="noopener">
          Or grab it from Releases.
        </a>
      </p>
      <div className="terminal reveal">
        <div className="term-head">
          <span>sentra</span>
          <button className="copy-btn" onClick={onCopy}>
            <Icon name="copy" size={13} />
            {copyLabel}
          </button>
        </div>
        <div className="term-body">
          <div>
            <span className="prompt">$ </span>
            {typed}
            <span className="cursor" />
          </div>
          <div className="term-out">{lines.join("\n")}</div>
        </div>
      </div>
    </section>
  );
}
