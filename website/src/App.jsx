import React, { useCallback, useEffect, useRef, useState } from "react";
import { CONFIG } from "./config";
import { useTheme } from "./useTheme";
import { CursorGlow } from "./hooks";
import Nav from "./components/Nav";
import Hero from "./components/Hero";
import CubeSection from "./components/CubeSection";
import StackSection from "./components/StackSection";
import Benchmarks from "./components/Benchmarks";
import Install from "./components/Install";
import Roadmap from "./components/Roadmap";
import Cta from "./components/Cta";
import Docs from "./pages/Docs";
import "./styles.css";

function formatStars(n) {
  return n >= 1000 ? (n / 1000).toFixed(1) + "k" : String(n);
}

function isDocsHash(hash) {
  return hash.startsWith("#/docs") || hash.startsWith("#docs");
}

export default function App() {
  const themers = useRef([]);
  const registerTheme = useCallback((fn) => {
    themers.current.push(fn);
  }, []);
  useTheme((t) => {
    themers.current.forEach((fn) => {
      try {
        fn(t);
      } catch {
        /* scene not ready */
      }
    });
  });

  const [route, setRoute] = useState(() => {
    return isDocsHash(window.location.hash) ? "docs" : "home";
  });

  useEffect(() => {
    const onHashChange = () => {
      const docs = isDocsHash(window.location.hash);
      const nextRoute = docs ? "docs" : "home";
      setRoute(nextRoute);

      if (!docs) {
        const targetId = window.location.hash.replace(/^#\/?/, "");
        if (targetId && targetId !== "top") {
          setTimeout(() => {
            const el = document.getElementById(targetId);
            if (el) el.scrollIntoView({ behavior: "smooth" });
          }, 60);
        } else {
          window.scrollTo({ top: 0, behavior: "smooth" });
        }
      }
    };
    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  const [stars, setStars] = useState("0");
  useEffect(() => {
    let alive = true;
    try {
      fetch("https://api.github.com/repos/" + CONFIG.GITHUB_REPO)
        .then((r) => (r.ok ? r.json() : null))
        .then((j) => {
          if (alive && j && typeof j.stargazers_count === "number") {
            setStars(formatStars(j.stargazers_count));
          }
        })
        .catch(() => {
          /* offline or rate-limited: leave placeholder */
        });
    } catch {
      /* no fetch: leave placeholder */
    }
    return () => {
      alive = false;
    };
  }, []);

  // cursor lean for both 3D scenes (desktop only)
  useEffect(() => {
    const fine = window.matchMedia("(pointer: fine)").matches;
    const calm = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    if (!fine || calm) return;
    let raf = 0;
    const move = (e) => {
      if (raf) return;
      raf = requestAnimationFrame(() => {
        raf = 0;
        window.dispatchEvent(
          new CustomEvent("sentra-lean", { detail: { x: e.clientX, y: e.clientY } })
        );
      });
    };
    window.addEventListener("mousemove", move, { passive: true });
    return () => window.removeEventListener("mousemove", move);
  }, []);

  return (
    <>
      <div className="bg-grid" />
      <div className="bg-glow" id="bg-glow" />
      <CursorGlow />
      <Nav starCount={stars} currentRoute={route} />
      {route === "docs" ? (
        <Docs />
      ) : (
        <div className="wrap" id="top">
          <Hero />
          <CubeSection registerTheme={registerTheme} />
          <StackSection registerTheme={registerTheme} />
          <Benchmarks />
          <Install />
          <Roadmap />
          <Cta starCount={stars} />
        </div>
      )}
    </>
  );
}
