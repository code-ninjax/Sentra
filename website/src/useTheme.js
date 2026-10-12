import { useCallback, useEffect, useState } from "react";

// One shared theme store for the whole app — every useTheme() call site
// reads and writes the same state, so the toggle can never desync.
// Default is "auto": the page follows the OS preference (and keeps
// following it live) until the user explicitly toggles.
const KEY = "sentra-theme";
const mq =
  typeof window !== "undefined" && window.matchMedia
    ? window.matchMedia("(prefers-color-scheme: light)")
    : null;

function read() {
  try {
    const s = localStorage.getItem(KEY);
    if (s === "light" || s === "dark") return s;
  } catch {
    /* private mode */
  }
  return "auto";
}

let current = read();
const subs = new Set();

function apply(t) {
  document.documentElement.setAttribute("data-theme", t);
}
apply(current);

export function effective(theme) {
  if (theme !== "auto") return theme;
  return mq && mq.matches ? "light" : "dark";
}

function emit(t) {
  subs.forEach((fn) => {
    try {
      fn(t);
    } catch {
      /* subscriber not ready */
    }
  });
}

function set(t) {
  current = t;
  try {
    if (t === "auto") localStorage.removeItem(KEY);
    else localStorage.setItem(KEY, t);
  } catch {
    /* private mode */
  }
  apply(t);
  emit(t);
}

// Live OS-level theme changes: notify scenes while in auto mode.
if (mq) {
  const onChange = () => {
    if (current === "auto") emit("auto");
  };
  if (mq.addEventListener) mq.addEventListener("change", onChange);
  else if (mq.addListener) mq.addListener(onChange);
}

export function useTheme(notify) {
  const [theme, setTheme] = useState(current);

  useEffect(() => {
    const fn = (t) => {
      setTheme(t);
      if (notify) notify(t);
    };
    subs.add(fn);
    return () => {
      subs.delete(fn);
    };
  }, [notify]);

  const toggle = useCallback(() => {
    set(effective(current) === "light" ? "dark" : "light");
  }, []);

  return [theme, toggle];
}
