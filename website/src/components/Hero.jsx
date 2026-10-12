import React, { useRef } from "react";
import { CONFIG } from "../config";
import { Icon } from "../icons";
import { useMagnetic, useReveal } from "../hooks";

function copyText(text, onDone) {
  const done = (ok) => onDone(ok ? "Copied" : "Failed");
  if (navigator.clipboard && navigator.clipboard.writeText) {
    navigator.clipboard.writeText(text).then(() => done(true), () => done(false));
  } else {
    const ta = document.createElement("textarea");
    ta.value = text;
    document.body.appendChild(ta);
    ta.select();
    try {
      done(document.execCommand("copy"));
    } catch {
      done(false);
    }
    document.body.removeChild(ta);
  }
}

// Hero: left-aligned editorial block — eyebrow, oversized headline,
// one-line pitch, a copyable command bar, star button, thin meta row.
export default function Hero() {
  const ref = useReveal();
  const cmdBar = useRef(null);
  const ghost = useRef(null);
  useMagnetic(cmdBar);
  useMagnetic(ghost);

  const shortCmd = CONFIG.INSTALL_CMD.split(" && ")[0] + " && …";

  const onCopy = () => {
    const label = cmdBar.current && cmdBar.current.querySelector(".cmd-copy-label");
    if (!label) return;
    const old = label.textContent;
    copyText(CONFIG.INSTALL_CMD, (msg) => {
      label.textContent = msg === "Copied" ? "Copied — see Install" : msg;
      setTimeout(() => {
        label.textContent = old;
      }, 1600);
    });
  };

  return (
    <header className="hero" ref={ref}>
      <div className="hero-eyebrow reveal mono">OCI CONTAINER RUNTIME — NO DAEMON</div>
      <h1 className="reveal">
        Containers,
        <br />
        <span className="accent">minus the daemon.</span>
      </h1>
      <p className="sub reveal">
        Open source, fast, lightweight, OCI-compliant. A container runtime in Go that does the
        work and exits.
      </p>
      <div className="cta-row reveal">
        <button className="cmd-bar" ref={cmdBar} onClick={onCopy} title="Click to copy">
          <span className="cmd-prompt mono">$</span>
          <code className="mono">{shortCmd}</code>
          <span className="cmd-copy">
            <Icon name="copy" size={14} />
            <span className="cmd-copy-label">Copy install command</span>
          </span>
        </button>
        <a className="btn btn-ghost" ref={ghost} href={CONFIG.GITHUB_URL} target="_blank" rel="noopener">
          <Icon name="star" size={16} />
          <span>Star on GitHub</span>
        </a>
      </div>
      <div className="badges reveal">
        <span className="badge">Apache 2.0</span>
        <span className="badge">Go</span>
        <span className="badge">
          <b>~3MB</b>&nbsp;packed
        </span>
      </div>
    </header>
  );
}
