import * as THREE from "three";
import { ICON_D } from "../iconPaths.js";

// Cube centerpiece: six slabs with gaps, glowing edges, canvas labels.
// Drag to rotate, hover to explode, click to lock. Reports hover so React
// can sync the feature panel.
export async function initCubeScene(opts) {
  const { canvas, placeholder, fallback, wiresEl, chips } = opts;
  const reduceMotion = opts.reduceMotion;
  const isMobile = opts.isMobile;

  const stage = canvas.parentElement;
  const W = () => stage.clientWidth;
  const H = () => (isMobile ? 360 : 480);

  const renderer = new THREE.WebGLRenderer({ canvas, alpha: true, antialias: true });
  renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, isMobile ? 1.5 : 2));
  // updateStyle=false: CSS (width:100%) owns the display size — no inline
  // px widths that could fight the layout on resize/orientation change
  renderer.setSize(W(), H(), false);

  const scene = new THREE.Scene();
  const camera = new THREE.PerspectiveCamera(42, W() / H(), 0.1, 100);
  camera.position.set(0, 0.6, 7.2);

  const isLight = () =>
    document.documentElement.getAttribute("data-theme") === "light" ||
    (document.documentElement.getAttribute("data-theme") === "auto" &&
      window.matchMedia("(prefers-color-scheme: light)").matches);
  const pal = () =>
    isLight()
      ? { edge: 0x16a34a, panel: 0xffffff, glow: 0x16a34a, label: "#16A34A", text: "#0A0A0A" }
      : { edge: 0x4ade80, panel: 0x111214, glow: 0x4ade80, label: "#4ADE80", text: "#FFFFFF" };
  let T = pal();

  scene.add(new THREE.AmbientLight(0xffffff, 0.55));
  const key = new THREE.DirectionalLight(0xffffff, 1.1);
  key.position.set(4, 6, 5);
  scene.add(key);
  const core = new THREE.PointLight(T.glow, 12, 12);
  scene.add(core);
  const rim = new THREE.DirectionalLight(0xffffff, 0.7);
  rim.position.set(-5, -2, -4);
  scene.add(rim);

  let particles = null;
  if (!reduceMotion) {
    const N = isMobile ? 140 : 320;
    const pos = new Float32Array(N * 3);
    for (let i = 0; i < N; i++) {
      pos[i * 3] = (Math.random() - 0.5) * 14;
      pos[i * 3 + 1] = (Math.random() - 0.5) * 9;
      pos[i * 3 + 2] = (Math.random() - 0.5) * 8 - 2;
    }
    const g = new THREE.BufferGeometry();
    g.setAttribute("position", new THREE.BufferAttribute(pos, 3));
    particles = new THREE.Points(
      g,
      new THREE.PointsMaterial({
        color: T.edge, size: 0.035, transparent: true, opacity: 0.55,
        blending: THREE.AdditiveBlending, depthWrite: false,
      })
    );
    scene.add(particles);
  }

  const cube = new THREE.Group();
  scene.add(cube);
  const GAP = 0.42, SIZE = 2.0, THICK = 0.34;
  const normals = [
    new THREE.Vector3(1, 0, 0), new THREE.Vector3(-1, 0, 0),
    new THREE.Vector3(0, 1, 0), new THREE.Vector3(0, -1, 0),
    new THREE.Vector3(0, 0, 1), new THREE.Vector3(0, 0, -1),
  ];
  const panels = [];

  function labelTexture(meta, accentCss, textCss) {
    const c = document.createElement("canvas");
    c.width = 512; c.height = 256;
    const x = c.getContext("2d");
    x.clearRect(0, 0, 512, 256);
    // per-face icon drawn from the shared path data (Path2D), 24-space scaled up
    const ds = (meta && meta.icon && ICON_D[meta.icon]) || ICON_D.box;
    const k = 4.6; // 24 * 4.6 = ~110px icon
    x.save();
    x.translate(256 - 12 * k, 100 - 12 * k);
    x.scale(k, k);
    x.strokeStyle = accentCss;
    x.lineWidth = 1.8;
    x.lineCap = "square";
    x.lineJoin = "miter";
    ds.forEach((d) => x.stroke(new Path2D(d)));
    x.restore();
    x.textAlign = "center";
    x.fillStyle = textCss;
    x.font = "600 42px system-ui, sans-serif";
    x.fillText((meta && meta.title ? meta.title : "Sentra").toUpperCase(), 256, 210);
    const t = new THREE.CanvasTexture(c);
    t.anisotropy = 4;
    return t;
  }

  normals.forEach((n, i) => {
    const meta = (opts.faces && opts.faces[i]) || { title: "Sentra" };
    const g = new THREE.Group();
    const geo = new THREE.BoxGeometry(
      n.x !== 0 ? THICK : SIZE, n.y !== 0 ? THICK : SIZE, n.z !== 0 ? THICK : SIZE);
    const mat = new THREE.MeshStandardMaterial({
      color: T.panel, roughness: 0.35, metalness: 0.55,
      transparent: true, opacity: 1, emissive: T.edge, emissiveIntensity: 0.06,
    });
    const mesh = new THREE.Mesh(geo, mat);
    mesh.userData.face = i;
    g.add(mesh);
    const edges = new THREE.LineSegments(
      new THREE.EdgesGeometry(geo),
      new THREE.LineBasicMaterial({ color: T.edge, transparent: true, opacity: 0.9 }));
    g.add(edges);
    const css = isLight()
      ? { accent: "#16A34A", text: "#0A0A0A" }
      : { accent: "#4ADE80", text: "#FFFFFF" };
    const label = new THREE.Mesh(
      new THREE.PlaneGeometry(SIZE * 0.92, SIZE * 0.46),
      new THREE.MeshBasicMaterial({
        map: labelTexture(meta, css.accent, css.text),
        transparent: true, opacity: 0.95, depthWrite: false,
      }));
    label.position.copy(n.clone().multiplyScalar(THICK / 2 + 0.012));
    if (n.x !== 0) label.rotation.y = n.x > 0 ? Math.PI / 2 : -Math.PI / 2;
    if (n.y !== 0) label.rotation.x = n.y > 0 ? -Math.PI / 2 : Math.PI / 2;
    if (n.z < 0) label.rotation.y = Math.PI;
    g.add(label);
    g.position.copy(n.clone().multiplyScalar(SIZE / 2 + GAP / 2 + THICK / 2 + 0.18));
    g.userData = { normal: n.clone(), base: g.position.clone(), edges, mat, label, meta };
    cube.add(g);
    panels.push(g);
  });

  let active = 0, locked = false, hovering = -1;
  let targetQuat = new THREE.Quaternion();
  let wantQuat = false;
  const ray = new THREE.Raycaster();
  const ptr = new THREE.Vector2(-2, -2);

  let dragging = false, px = 0, py = 0, vx = 0, vy = 0;
  const autoSpin = !reduceMotion;
  canvas.addEventListener("pointerdown", (e) => {
    dragging = true; px = e.clientX; py = e.clientY;
    try { canvas.setPointerCapture(e.pointerId); } catch { /* noop */ }
    locked = false; wantQuat = false;
  });
  canvas.addEventListener("pointermove", (e) => {
    const r = canvas.getBoundingClientRect();
    ptr.x = ((e.clientX - r.left) / r.width) * 2 - 1;
    ptr.y = -((e.clientY - r.top) / r.height) * 2 + 1;
    if (!dragging) return;
    vx = (e.clientX - px) * 0.005;
    vy = (e.clientY - py) * 0.005;
    cube.rotation.y += vx;
    cube.rotation.x += vy;
    px = e.clientX; py = e.clientY;
  });
  const endDrag = () => { dragging = false; };
  canvas.addEventListener("pointerup", endDrag);
  canvas.addEventListener("pointercancel", endDrag);
  canvas.addEventListener("click", () => {
    if (hovering >= 0 && opts.onSelect) opts.onSelect(hovering);
  });

  const api = {
    setActive(i, lock) {
      active = i;
      if (!lock) return;
      locked = true;
      const n = panels[i].userData.normal.clone().applyQuaternion(cube.quaternion).normalize();
      targetQuat = new THREE.Quaternion().setFromUnitVectors(n, new THREE.Vector3(0, 0, 1));
      wantQuat = true;
    },
    lean(gx, gy) {
      if (dragging || locked) return;
      const r = canvas.getBoundingClientRect();
      const dx = (gx - (r.left + r.width / 2)) / r.width;
      const dy = (gy - (r.top + r.height / 2)) / r.height;
      cube.rotation.y += dx * 0.007;
      cube.rotation.x += dy * 0.005;
    },
    retheme() {
      T = pal();
      core.color.setHex(T.glow);
      const css = isLight()
        ? { accent: "#16A34A", text: "#0A0A0A" }
        : { accent: "#4ADE80", text: "#FFFFFF" };
      panels.forEach((p) => {
        p.userData.mat.color.setHex(T.panel);
        p.userData.mat.emissive.setHex(T.edge);
        p.userData.edges.material.color.setHex(T.edge);
        const old = p.userData.label.material.map;
        p.userData.label.material.map = labelTexture(
          p.userData.meta, css.accent, css.text);
        p.userData.label.material.needsUpdate = true;
        if (old) old.dispose();
      });
      if (particles) particles.material.color.setHex(T.edge);
    },
    dispose() {
      renderer.setAnimationLoop(null);
      scene.traverse((o) => {
        if (o.geometry) o.geometry.dispose();
        if (o.material) {
          (Array.isArray(o.material) ? o.material : [o.material]).forEach((m) => {
            if (m.map) m.map.dispose();
            m.dispose();
          });
        }
      });
      renderer.dispose();
    },
  };

  const layoutEl = () => document.getElementById("cube-layout");
  function drawWire() {
    if (isMobile || !wiresEl) return;
    const chip = chips[active];
    if (!chip) return;
    const sr = layoutEl().getBoundingClientRect();
    wiresEl.setAttribute("viewBox", `0 0 ${sr.width} ${sr.height}`);
    const anchor = new THREE.Vector3();
    panels[active].userData.label.getWorldPosition(anchor);
    const v = anchor.clone().project(camera);
    const r = canvas.getBoundingClientRect();
    const a = {
      x: (v.x * 0.5 + 0.5) * r.width + (r.left - sr.left),
      y: (-v.y * 0.5 + 0.5) * r.height + (r.top - sr.top),
    };
    const cr = chip.getBoundingClientRect();
    const b = { x: cr.left - sr.left, y: cr.top - sr.top + cr.height / 2 };
    const mx = (a.x + b.x) / 2;
    wiresEl.innerHTML =
      `<path d="M ${a.x} ${a.y} C ${mx} ${a.y}, ${mx} ${b.y}, ${b.x} ${b.y}" ` +
      `fill="none" stroke="currentColor" stroke-width="1.5" ` +
      `style="color:var(--accent);opacity:0.8"/>` +
      `<circle cx="${a.x}" cy="${a.y}" r="3" style="fill:var(--accent)"/>`;
  }

  const clock = new THREE.Clock();
  let visible = true;
  const visObs = new IntersectionObserver((es) => {
    visible = es[0].isIntersecting;
  });
  visObs.observe(canvas);
  const onVis = () => {
    if (document.hidden) renderer.setAnimationLoop(null);
    else renderer.setAnimationLoop(tick);
  };
  document.addEventListener("visibilitychange", onVis);
  const ro = new ResizeObserver(() => {
    const h = H();
    camera.aspect = W() / h;
    camera.updateProjectionMatrix();
    renderer.setSize(W(), h, false);
  });
  ro.observe(stage);

  if (placeholder) placeholder.classList.add("hidden");

  function tick() {
    if (!visible || document.hidden) return;
    const t = clock.getElapsedTime();
    if (autoSpin && !dragging && !locked) cube.rotation.y += 0.0035;
    if (!dragging && (Math.abs(vx) > 0.0004 || Math.abs(vy) > 0.0004)) {
      cube.rotation.y += vx;
      cube.rotation.x += vy;
      vx *= 0.95;
      vy *= 0.95;
    }
    cube.position.y = reduceMotion ? 0 : Math.sin(t * 0.8) * 0.08;
    if (wantQuat) {
      const cur = new THREE.Quaternion();
      cube.getWorldQuaternion(cur);
      cur.slerp(targetQuat, 0.1);
      cube.quaternion.copy(cur);
      if (cur.angleTo(targetQuat) < 0.02) wantQuat = false;
    }
    ray.setFromCamera(ptr, camera);
    const hits = ray.intersectObjects(panels.map((p) => p.children[0]));
    const next = hits.length ? hits[0].object.userData.face : -1;
    if (next !== hovering) {
      hovering = next;
      if (opts.onHover) opts.onHover(hovering);
    }
    panels.forEach((p, i) => {
      const u = p.userData;
      const on = i === active || i === hovering;
      const target = u.base.clone().add(u.normal.clone().multiplyScalar(on ? 0.55 : 0));
      p.position.lerp(target, 0.14);
      u.edges.material.opacity = on ? 1 : 0.55;
      u.mat.emissiveIntensity = i === active ? 0.28 : i === hovering ? 0.16 : 0.06;
      const dim = !on && hovering >= 0;
      u.mat.opacity = dim ? 0.55 : 1;
      u.label.material.opacity = dim ? 0.5 : 0.95;
    });
    if (particles && !reduceMotion) {
      particles.rotation.y = t * 0.02;
      core.intensity = 12 + Math.sin(t * 1.6) * 2;
    }
    renderer.render(scene, camera);
    drawWire();
  }
  renderer.setAnimationLoop(tick);

  api.dispose = ((prev) => () => {
    visObs.disconnect();
    ro.disconnect();
    document.removeEventListener("visibilitychange", onVis);
    prev();
  })(api.dispose);

  return api;
}
