import * as THREE from "three";

// "Works with your stack": isometric 3x3 tile grid floating over layered
// translucent slabs. Hover raises a tile, click selects it. Clean and quiet.
export async function initStackScene(opts) {
  const { canvas } = opts;
  const reduceMotion = opts.reduceMotion;
  const isMobile = opts.isMobile;
  const items = opts.items || [];

  const stage = canvas.parentElement;
  const W = () => stage.clientWidth;
  const H = () => (isMobile ? 360 : 480);

  const renderer = new THREE.WebGLRenderer({ canvas, alpha: true, antialias: true });
  renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, isMobile ? 1.5 : 2));
  // updateStyle=false: CSS owns display size; avoids inline px overflow
  renderer.setSize(W(), H(), false);

  const scene = new THREE.Scene();
  const camera = new THREE.PerspectiveCamera(36, W() / H(), 0.1, 100);
  camera.position.set(6.4, 6.0, 6.4);
  camera.lookAt(0, -0.6, 0);

  const isLight = () =>
    document.documentElement.getAttribute("data-theme") === "light" ||
    (document.documentElement.getAttribute("data-theme") === "auto" &&
      window.matchMedia("(prefers-color-scheme: light)").matches);
  const pal = () =>
    isLight()
      ? { edge: 0x16a34a, tile: 0xf2f2ef, slab: 0x16a34a, text: "#0A0A0A" }
      : { edge: 0x4ade80, tile: 0x141619, slab: 0x4ade80, text: "#FFFFFF" };
  let T = pal();

  scene.add(new THREE.AmbientLight(0xffffff, 0.7));
  const key = new THREE.DirectionalLight(0xffffff, 1.2);
  key.position.set(5, 9, 4);
  scene.add(key);
  const under = new THREE.PointLight(T.slab, 8, 16);
  under.position.set(0, -2.4, 0);
  scene.add(under);

  const world = new THREE.Group();
  scene.add(world);

  // stacked translucent slabs beneath
  const slabMats = [];
  const SLABS = 5;
  for (let s = 0; s < SLABS; s++) {
    const g = new THREE.BoxGeometry(7.6 - s * 0.28, 0.16, 7.6 - s * 0.28);
    const m = new THREE.MeshStandardMaterial({
      color: T.tile, roughness: 0.4, metalness: 0.5,
      transparent: true, opacity: 0.32 - s * 0.035,
    });
    const mesh = new THREE.Mesh(g, m);
    mesh.position.y = -1.5 - s * 0.34;
    const e = new THREE.LineSegments(
      new THREE.EdgesGeometry(g),
      new THREE.LineBasicMaterial({ color: T.slab, transparent: true, opacity: 0.5 - s * 0.06 }));
    mesh.add(e);
    world.add(mesh);
    slabMats.push({ m, e: e.material });
  }

  // Real devicon SVG (asset URL from the devicon package) drawn onto a
  // canvas texture. Loads async; the transparent texture fills in on load.
  function iconTexture(url) {
    const c = document.createElement("canvas");
    c.width = 256; c.height = 256;
    const t = new THREE.CanvasTexture(c);
    t.anisotropy = 4;
    if (!url) return t;
    const img = new Image();
    img.onload = () => {
      const x = c.getContext("2d");
      x.clearRect(0, 0, 256, 256);
      x.drawImage(img, 48, 48, 160, 160);
      t.needsUpdate = true;
    };
    img.src = url;
    return t;
  }

  // 3x3 tiles
  const tiles = [];
  const SP = 2.15, TS = 1.7, TH = 0.22;
  items.forEach((item, i) => {
    const cx = (i % 3) - 1, cz = Math.floor(i / 3) - 1;
    const g = new THREE.Group();
    const geo = new THREE.BoxGeometry(TS, TH, TS);
    const mat = new THREE.MeshStandardMaterial({
      color: T.tile, roughness: 0.35, metalness: 0.55,
      transparent: true, opacity: 0.96, emissive: T.slab, emissiveIntensity: 0.05,
    });
    const mesh = new THREE.Mesh(geo, mat);
    mesh.userData.stack = i;
    g.add(mesh);
    const edges = new THREE.LineSegments(
      new THREE.EdgesGeometry(geo),
      new THREE.LineBasicMaterial({ color: T.slab, transparent: true, opacity: 0.85 }));
    g.add(edges);
    const top = new THREE.Mesh(
      new THREE.PlaneGeometry(TS * 0.86, TS * 0.86),
      new THREE.MeshBasicMaterial({
        map: iconTexture(item.iconUrl),
        transparent: true, depthWrite: false,
      }));
    top.rotation.x = -Math.PI / 2;
    top.position.y = TH / 2 + 0.012;
    g.add(top);
    const lift = 0.55 * (1 - (Math.abs(cx) + Math.abs(cz)) / 4);
    g.position.set(cx * SP, 0.4 + lift, cz * SP);
    g.userData = { base: g.position.clone(), mat, edges, top, meta: item };
    world.add(g);
    tiles.push(g);
  });

  let selected = 4, hovering = -1;
  const ray = new THREE.Raycaster();
  const ptr = new THREE.Vector2(-2, -2);
  let dragging = false, px = 0, spin = 0;

  canvas.addEventListener("pointerdown", (e) => {
    dragging = true; px = e.clientX;
    try { canvas.setPointerCapture(e.pointerId); } catch { /* noop */ }
  });
  canvas.addEventListener("pointermove", (e) => {
    const r = canvas.getBoundingClientRect();
    ptr.x = ((e.clientX - r.left) / r.width) * 2 - 1;
    ptr.y = -((e.clientY - r.top) / r.height) * 2 + 1;
    if (!dragging) return;
    spin = (e.clientX - px) * 0.004;
    world.rotation.y += spin;
    px = e.clientX;
  });
  const endDrag = () => { dragging = false; };
  canvas.addEventListener("pointerup", endDrag);
  canvas.addEventListener("pointercancel", endDrag);
  canvas.addEventListener("click", () => {
    if (hovering >= 0) {
      selected = hovering;
      if (opts.onSelect) opts.onSelect(selected);
    }
  });

  const api = {
    select(i) {
      selected = i;
      if (opts.onSelect) opts.onSelect(i);
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
      slabMats.forEach((s) => {
        s.m.color.setHex(T.tile);
        s.e.color.setHex(T.slab);
      });
      tiles.forEach((t) => {
        t.userData.mat.color.setHex(T.tile);
        t.userData.mat.emissive.setHex(T.slab);
        t.userData.edges.material.color.setHex(T.slab);
        // top face is a colored devicon — no recolor needed on theme change
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
    renderer.setSize(W(), h, false);
  });
  ro.observe(stage);

  function tick() {
    if (!visible || document.hidden) return;
    const t = clock.getElapsedTime();
    if (!dragging) {
      spin *= 0.95;
      world.rotation.y += spin + (reduceMotion ? 0 : 0.0018);
    }
    ray.setFromCamera(ptr, camera);
    const hits = ray.intersectObjects(tiles.map((g) => g.children[0]));
    hovering = hits.length ? hits[0].object.userData.stack : -1;
    tiles.forEach((g, i) => {
      const u = g.userData;
      const on = i === selected || i === hovering;
      const target = u.base.clone();
      target.y += on ? 0.5 : reduceMotion ? 0 : Math.sin(t * 1.1 + i) * 0.06;
      g.position.lerp(target, 0.12);
      u.edges.material.opacity = on ? 1 : 0.7;
      u.mat.emissiveIntensity = i === selected ? 0.3 : i === hovering ? 0.18 : 0.05;
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
