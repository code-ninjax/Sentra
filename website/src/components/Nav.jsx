import React, { useEffect, useState } from "react";
import { CONFIG } from "../config";
import { Icon, LogoMark } from "../icons";
import { effective, useTheme } from "../useTheme";

const LINKS = [
  ["Features", "#features"],
  ["Benchmarks", "#benchmarks"],
  ["Install", "#install"],
  ["Roadmap", "#roadmap"],
];

// One fixed nav that morphs on scroll: full bar at top, compact floating
// bar (wordmark + Docs + star only) once past the hero.
export default function Nav({ starCount }) {
  const [theme, toggle] = useTheme();
  const [compact, setCompact] = useState(false);
  const [active, setActive] = useState("");

  useEffect(() => {
    const onScroll = () => {
      setCompact(window.scrollY > window.innerHeight * 0.55);
      let current = "";
      ["features", "benchmarks", "install", "roadmap"].forEach((id) => {
        const el = document.getElementById(id);
        if (el && el.getBoundingClientRect().top < window.innerHeight * 0.4) current = id;
      });
      setActive(current);
    };
    window.addEventListener("scroll", onScroll, { passive: true });
    onScroll();
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  const isLight = effective(theme) === "light";

  return (
    <nav className={"nav" + (compact ? " compact" : "")} aria-label="Main navigation">
      <div className="nav-inner">
        <a className="wordmark" href="#top" aria-label="Sentra home">
          <LogoMark size={compact ? 22 : 26} />
          Sentra
        </a>
        <div className="nav-full">
          <div className="nav-links">
            {LINKS.map(([label, href]) => (
              <a key={href} href={href} className={active === href.slice(1) ? "active" : ""}>
                {label}
              </a>
            ))}
            <a href={CONFIG.DOCS_URL} target="_blank" rel="noopener">
              Docs
            </a>
          </div>
          <div className="nav-actions">
            <button className="icon-btn" onClick={toggle} aria-label="Toggle color theme">
              <Icon name={isLight ? "moon" : "sun"} size={18} />
            </button>
            <a className="icon-btn star" href={CONFIG.GITHUB_URL} target="_blank" rel="noopener">
              <Icon name="star" size={16} />
              <span>{starCount}</span>
            </a>
          </div>
        </div>
        <div className="nav-compact-only">
          <a className="nav-docs" href={CONFIG.DOCS_URL} target="_blank" rel="noopener">
            Docs
          </a>
          <a
            className="icon-btn star"
            href={CONFIG.GITHUB_URL}
            target="_blank"
            rel="noopener"
            aria-label="Star Sentra on GitHub"
          >
            <Icon name="star" size={16} />
            <span>{starCount}</span>
          </a>
        </div>
      </div>
    </nav>
  );
}
