import React from "react";
import { ICON_D } from "./iconPaths";

// Single SVG icon set. Geometry lives in iconPaths.js so the three.js
// scenes can draw the same icons on canvas. No emojis anywhere.
// stroke="currentColor" so icons inherit text color. Sharp corners only.
function Svg({ size = 22, children, ...rest }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.8}
      strokeLinecap="square"
      strokeLinejoin="miter"
      aria-hidden="true"
      {...rest}
    >
      {children}
    </svg>
  );
}

export function Icon({ name, size }) {
  const ds = ICON_D[name] || ICON_D.box;
  return (
    <Svg size={size}>
      {ds.map((d, i) => (
        <path key={i} d={d} />
      ))}
    </Svg>
  );
}

export function LogoMark({ size = 26 }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 32 32"
      aria-hidden="true"
      style={{ flexShrink: 0, display: "block" }}
    >
      <rect x="4" y="4" width="24" height="24" fill="none" stroke="currentColor" strokeWidth="2.5" />
      <circle cx="16" cy="16" r="5.5" fill="#4ADE80" />
    </svg>
  );
}
