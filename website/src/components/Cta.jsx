import React from "react";
import { CONFIG } from "../config";
import { Icon } from "../icons";
import { useReveal } from "../hooks";

// Slim closing banner: heading, star button with live count, contribute link.
export default function Cta({ starCount }) {
  const ref = useReveal();
  return (
    <>
      <section aria-label="Contribute" ref={ref} style={{ paddingBottom: 0 }}>
        <div className="cta-slim reveal">
          <div>
            <h2>Star Sentra on GitHub</h2>
            <p>
              Built in public.{" "}
              <span className="mono" style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
                <Icon name="star" size={13} /> {starCount}
              </span>{" "}
              and counting.
            </p>
          </div>
          <div className="cta-row">
            <a className="btn btn-primary" href={CONFIG.GITHUB_URL} target="_blank" rel="noopener">
              <Icon name="star" size={16} />
              <span>Star on GitHub</span>
            </a>
            <a className="btn btn-ghost" href={CONFIG.GITHUB_URL} target="_blank" rel="noopener">
              <span>Contribute</span>
            </a>
          </div>
        </div>
      </section>
      <footer>
        <span>
          built by{" "}
          <a href="https://github.com/code-ninjax" target="_blank" rel="noopener">
            devbeliever
          </a>
        </span>
        <span>
          <a href={CONFIG.GITHUB_URL + "/blob/main/LICENSE"} target="_blank" rel="noopener">
            Apache 2.0
          </a>{" "}
          · <a href={CONFIG.GITHUB_URL} target="_blank" rel="noopener">GitHub</a>
        </span>
      </footer>
    </>
  );
}
