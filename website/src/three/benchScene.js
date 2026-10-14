import * as THREE from "three";

// Interactive 3D benchmark chart: two bar clusters (startup, memory),
// sentra solid-accent vs docker ghost. Drag to orbit, hover or tap a bar
// to inspect it. Zero values render as TBD ghost stubs — never faked.
export async function initBenchScene(opts) {
  const { canvas } = opts;
  const reduceMotion = opts.reduceMotion;
  const isMobile = opts.isMobile;
  const bars = opts.bars || []; // [{metric, runtime, value, unit, n, stamp}]

  const stage = canvas.parentElement;
  const W = () => stage.clientWidth;
  const H = () => (isMobile ? 360 : 480);

  const renderer = new THREE.WebGLRenderer({ canvas, alpha: true, antialias: true });
  renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, isMobile ? 1.5 : 2));
  // updateStyle=false: CSS owns display size; avoids inline px overflow
  renderer.setSize(W(), H(), false);

  const scene = new THREE.Scene();
  const camera = new THREE.PerspectiveCamera(40, W() / H(), 0.1, 100);
  // Frame both clusters with room for labels; pull back on narrow screens.
  const frameCamera = () => {
    const a = W() / H();
    camera.position.set(0, 4.8, a < 1 ? 14.5 : 11.8);
    camera.lookAt(0, 1.0, 0);
  };
  frameCamera();

  const isLight = () =>
    document.documentElement.getAttribute("data-theme") === "light" ||
    (document.documentElement.getAttribute("data-theme") === "auto" &&
      window.matchMedia("(prefers-color-scheme: light)").matches);
  const pal = () =>
    isLight()
      ? { edge: 0x16a34a, tile: 0xf2f2ef, slab: 0x16a34a, faint: 0x9a9a9a }
      : { edge: 0x4ade80, tile: 0x141619, slab: 0x4ade80, faint: 0x5a5a5a };
  let T = pal();
  const css = () =>
    isLight()
      ? { accent: "#16A34A", text: "#0A0A0A", muted: "#5A5A5A" }
      : { accent: "#4ADE80", text: "#FFFFFF", muted: "#9A9A9A" };

  scene.add(new THREE.AmbientLight(0xffffff, 0.65));
  const key = new THREE.DirectionalLight(0xffffff, 1.15);
  key.position.set(5, 9, 5);
  scene.add(key);
  const under = new THREE.PointLight(T.slab, 9, 18);
  under.position.set(0, -1.6, 2);
  scene.add(under);

  const world = new THREE.Group();
  scene.add(world);

  function labelSprite(lines) {
    // lines: [{text, color, size, weight}]
    const c = document.createElement("canvas");
    c.width = 512; c.height = 256;
    const x = c.getContext("2d");
    x.clearRect(0, 0, 512, 256);
    x.textAlign = "center";
    let y = 0;
    lines.forEach((ln) => {
      y += ln.size * 1.35;
      x.fillStyle = ln.color;
      x.font = `${ln.weight || 600} ${ln.size}px system-ui, sans-serif`;
      x.fillText(ln.text, 256, y);
    });
    const t = new THREE.CanvasTexture(c);
    t.anisotropy = 4;
    const sp = new THREE.Sprite(
      new THREE.SpriteMaterial({ map: t, transparent: true, depthWrite: false }));
    sp.scale.set(2.7, 1.35, 1);
    return sp;
  }

  function valueLabel(meta) {
    const C = css();
    const main = meta.value > 0 ? `${meta.value}${meta.unit}` : "TBD";
    const mainColor = meta.value <= 0 || meta.runtime === "docker" ? C.muted : C.accent;
    return labelSprite([
      { text: main, color: mainColor, size: 84, weight: 700 },
      { text: `${meta.runtime} · n=${meta.n}`, color: C.muted, size: 44 },
    ]);
  }

  // cluster max per metric (epsilon so a lone TBD still draws stubs)
  const maxOf = (metric) =>
    Math.max(0.0001, ...bars.filter((b) => b.metric === metric).map((b) => b.value));
  const barH = (v, max) => (v <= 0 ? 0.5 : 0.5 + (v / max) * 3.0);

  const meshes = []; // {mesh, edges, label, meta, base}
  const clusters = [
    { metric: "Startup time", x: -2.5, title: "STARTUP", sub: "lower is better" },
    { metric: "Memory footprint", x: 2.5, title: "MEMORY", sub: "lower is better" },
  ];

  function titleSprite(cl) {
    const C = css();
    return labelSprite([
      { text: cl.title, color: C.muted, size: 72, weight: 700 },
      { text: cl.sub, color: C.muted, size: 44 },
    ]);
  }

  clusters.forEach((cl) => {
    const g = new THREE.Group();
    g.position.x = cl.x;
    const max = maxOf(cl.metric);
    // slab
    const slabGeo = new THREE.BoxGeometry(3.5, 0.18, 2.3);
    const slabMat = new THREE.MeshStandardMaterial({
      color: T.tile, roughness: 0.4, metalness: 0.5, transparent: true, opacity: 0.9,
    });
    const slab = new THREE.Mesh(slabGeo, slabMat);
    slab.position.y = -0.09;
    g.add(slab);
    const slabEdges = new THREE.LineSegments(
      new THREE.EdgesGeometry(slabGeo),
      new THREE.LineBasicMaterial({ color: T.slab, transparent: true, opacity: 0.6 }));
    slab.add(slabEdges);
    g.userData.slab = { mat: slabMat, edges: slabEdges.material };

    bars
      .map((b, i) => ({ b, i }))
      .filter(({ b }) => b.metric === cl.metric)
      .forEach(({ b, i }, k) => {
        const bx = k === 0 ? -0.85 : 0.85;
        const h = barH(b.value, max);
        const geo = new THREE.BoxGeometry(0.95, 1, 0.95);
        const tbd = b.value <= 0;
        const isDocker = b.runtime === "docker";
        const baseColor = tbd ? T.tile : isDocker ? T.faint : T.edge;
        const mat = new THREE.MeshStandardMaterial({
          color: baseColor,
          roughness: 0.35, metalness: 0.4,
          transparent: true, opacity: tbd ? 0.18 : isDocker ? 0.85 : 0.92,
          emissive: tbd ? T.faint : baseColor, emissiveIntensity: tbd ? 0.08 : 0.12,
        });
        const mesh = new THREE.Mesh(geo, mat);
        mesh.scale.y = h;
        mesh.position.set(bx, h / 2, 0);
        mesh.userData.bar = i;
        g.add(mesh);
        const edges = new THREE.LineSegments(
          new THREE.EdgesGeometry(geo),
          new THREE.LineBasicMaterial({
            color: tbd ? T.faint : baseColor, transparent: true, opacity: tbd ? 0.8 : 0.95,
          }));
        edges.scale.y = h;
        edges.position.copy(mesh.position);
        g.add(edges);
        const label = valueLabel(b);
        label.position.set(bx, h + 0.95, 0);
        g.add(label);
        meshes.push({ mesh, edges, label, meta: b, slab: g.userData.slab });
      });

    const title = titleSprite(cl);
    title.position.set(0, -0.55, 1.7);
    title.scale.set(2.7, 1.35, 1);
    g.add(title);
    g.userData.title = title;
    g.userData.titleText = cl.title;
    g.userData.titleSub = cl.sub;
    world.add(g);
  });

  let selected = 0, hovering = -1;
  const ray = new THREE.Raycaster();
  const ptr = new THREE.Vector2(-2, -2);
  let dragging = false, px = 0, py = 0;

  const emit = (i) => {
    if (i >= 0 && opts.onSelect) opts.onSelect(i);
  };

  canvas.addEventListener("pointerdown", (e) => {
    dragging = true; px = e.clientX; py = e.clientY;
    try { canvas.setPointerCapture(e.pointerId); } catch { /* noop */ }
  });
  canvas.addEventListener("pointermove", (e) => {
    const r = canvas.getBoundingClientRect();
    ptr.x = ((e.clientX - r.left) / r.width) * 2 - 1;
    ptr.y = -((e.clientY - r.top) / r.height) * 2 + 1;
    if (!dragging) return;
    world.rotation.y += (e.clientX - px) * 0.006;
    world.rotation.x = Math.max(-0.15, Math.min(0.55, world.rotation.x + (e.clientY - py) * 0.003));
    px = e.clientX; py = e.clientY;
  });
  const endDrag = () => { dragging = false; };
  canvas.addEventListener("pointerup", endDrag);
  canvas.addEventListener("pointercancel", endDrag);
  canvas.addEventListener("click", () => {
    if (hovering >= 0) {
      selected = hovering;
      emit(selected);
    }
  });

  const api = {
    select(i) {
      if (i < 0 || i >= meshes.length) return;
      selected = i;
      emit(selected);
    },
    lean(gx, gy) {
      if (dragging) return;
      const r = canvas.getBoundingClientRect();
      const dx = (gx - (r.left + r.width / 2)) / r.width;
      world.rotation.y += dx * 0.004;
    },
    retheme() {
      T = pal();
      under.color.setHex(T.slab);
      meshes.forEach((m) => {
        const tbd = m.meta.value <= 0;
        const isDocker = m.meta.runtime === "docker";
        const baseColor = tbd ? T.tile : isDocker ? T.faint : T.edge;
        m.mesh.material.color.setHex(baseColor);
        m.mesh.material.emissive.setHex(tbd ? T.faint : baseColor);
        m.edges.material.color.setHex(tbd ? T.faint : baseColor);
        m.slab.mat.color.setHex(T.tile);
        m.slab.edges.color.setHex(T.slab);
        const old = m.label.material.map;
        const fresh = valueLabel(m.meta);
        m.label.material.map = fresh.material.map;
        m.label.material.needsUpdate = true;
        old.dispose();
        fresh.material.dispose();
      });
      world.children.forEach((g) => {
        if (!g.userData.title) return;
        const old = g.userData.title.material.map;
        const fresh = titleSprite({ title: g.userData.titleText, sub: g.userData.titleSub });
        g.userData.title.material.map = fresh.material.map;
        g.userData.title.material.needsUpdate = true;
        old.dispose();
        fresh.material.dispose();
      });
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
    frameCamera();
    renderer.setSize(W(), h, false);
  });
  ro.observe(stage);

  function tick() {
    if (!visible || document.hidden) return;
    const t = clock.getElapsedTime();
    if (!dragging && !reduceMotion) world.rotation.y += 0.0016;
    ray.setFromCamera(ptr, camera);
    const hits = ray.intersectObjects(meshes.map((m) => m.mesh));
    const next = hits.length ? hits[0].object.userData.bar : -1;
    if (next !== hovering) {
      hovering = next;
      if (opts.onHover) opts.onHover(hovering);
    }
    meshes.forEach((m, i) => {
      const on = i === selected || i === hovering;
      const target = on ? 0.35 : 0;
      m.mesh.position.y += ((m.mesh.scale.y / 2 + target) - m.mesh.position.y) * 0.15;
      m.edges.position.y = m.mesh.position.y;
      m.label.position.y += ((m.mesh.scale.y + 0.95 + (on ? 0.35 : 0)) - m.label.position.y) * 0.15;
      m.mesh.material.emissiveIntensity = i === selected ? 0.35 : i === hovering ? 0.25 : (m.meta.value <= 0 ? 0.08 : 0.12);
    });
    renderer.render(scene, camera);
  }
  renderer.setAnimationLoop(tick);

  const prevDispose = api.dispose;
  api.dispose = () => {
    visObs.disconnect();
    ro.disconnect();
    document.removeEventListener("visibilitychange", onVis);
    prevDispose();
  };
  return api;
}
