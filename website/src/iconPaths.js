// Single source of truth for icon geometry: SVG path-data strings only,
// no React. Consumed by icons.jsx (render) and the three.js scenes
// (canvas texture via Path2D). Keep in the 0 0 24 24 coordinate space.
export const ICON_D = {
  star: ["M12 3l2.7 5.6 6.1.8-4.5 4.2 1.1 6-5.4-3-5.4 3 1.1-6L3.2 9.4l6.1-.8L12 3z"],
  sun: [
    "M16 12a4 4 0 1 1-8 0 4 4 0 1 1 8 0",
    "M12 2v3M12 19v3M2 12h3M19 12h3M4.5 4.5l2 2M17.5 17.5l2 2M19.5 4.5l-2 2M6.5 17.5l-2 2",
  ],
  moon: ["M20 14.5A8 8 0 0 1 9.5 4 8 8 0 1 0 20 14.5z"],
  chevL: ["M14 5l-7 7 7 7"],
  chevR: ["M10 5l7 7-7 7"],
  copy: ["M9 9h12v12H9z", "M5 15V3h12"],
  check: ["M4 12.5l5 5L20 6.5"],
  layers: ["M12 3l9 5-9 5-9-5 9-5z", "M3 13l9 5 9-5"],
  zap: ["M13 2L4 14h6l-1 8 9-12h-6l1-8z"],
  shield: ["M12 3l7 3v6c0 4.5-3 7.5-7 9-4-1.5-7-4.5-7-9V6l7-3z", "M9 12l2 2 4-4"],
  box: ["M3 8l9-5 9 5v8l-9 5-9-5V8z", "M3 8l9 5 9-5M12 13v8"],
  feather: ["M20 4c-6 0-12 4-14 12l-2 4 4-2c8-2 12-8 12-14zM6 18l9-9"],
  sparkle: [
    "M12 3l1.8 5.2L19 10l-5.2 1.8L12 17l-1.8-5.2L5 10l5.2-1.8L12 3z",
    "M19 15l.9 2.1L22 18l-2.1.9L19 21l-.9-2.1L16 18l2.1-.9L19 15z",
  ],
  camera: [
    "M3 7h18v13H3z",
    "M8 7l1.5-3h5L16 7",
    "M15.5 13a3.5 3.5 0 1 1-7 0 3.5 3.5 0 1 1 7 0",
  ],
  activity: ["M3 12h4l3 8 4-16 3 8h4"],
  terminal: ["M3 4h18v16H3z", "M7 9l3 3-3 3M12 15h5"],
  monitor: ["M3 5h18v12H3z", "M12 17v4M9 21h6"],
  server: [
    "M3 4h18v7H3z",
    "M3 13h18v7H3z",
    "M7 7.5h.01M7 16.5h.01",
  ],
};
