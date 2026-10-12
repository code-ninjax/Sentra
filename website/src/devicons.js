// Real stack logos from the devicon package (npm i devicon), bundled by
// Vite as asset URLs. Used in the stack chips and as 3D tile textures.
import node from "devicon/icons/nodejs/nodejs-original.svg";
import python from "devicon/icons/python/python-original.svg";
import go from "devicon/icons/go/go-original-wordmark.svg";
import rust from "devicon/icons/rust/rust-original.svg";
import docker from "devicon/icons/docker/docker-original.svg";
import postgres from "devicon/icons/postgresql/postgresql-original.svg";
import redis from "devicon/icons/redis/redis-original.svg";
import bun from "devicon/icons/bun/bun-original.svg";
import express from "devicon/icons/express/express-original.svg";

export const DEVICON = {
  "Node.js": node,
  Python: python,
  Go: go,
  Rust: rust,
  Docker: docker,
  Postgres: postgres,
  Redis: redis,
  Bun: bun,
  Express: express,
};

export function deviconFor(name) {
  return DEVICON[name] || null;
}
