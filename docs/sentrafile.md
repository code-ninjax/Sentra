# Sentrafile Syntax Reference

A `Sentrafile` is the recipe for building a container image with Sentra. It is clean, predictable, and has no hidden shell wrappers.

---

## The Docker-to-Sentra Cheat Sheet

If you already know how to write a `Dockerfile`, you already know 90% of a `Sentrafile`:

| Docker Directive | Sentra Directive | What it does | Example |
|---|---|---|---|
| `FROM` | `base` | Chooses the starting image | `base node:20-alpine` |
| `WORKDIR` | `workdir` | Sets the folder inside the container | `workdir /app` |
| `COPY` | `copy` | Copies files from your machine into the container | `copy package.json ./` |
| `RUN` | `exec` | Runs a command during build time | `exec npm install` |
| `ENV` | `env` | Sets environment variables | `env PORT=8080 NODE_ENV=production` |
| `EXPOSE` | `expose` | Documents network ports | `expose 8080` |
| `CMD` / `ENTRYPOINT` | `start` | The command that starts when container runs | `start node server.js` |

---

## Directives in Detail

### 1. `base <image> [as <name>]`
Specifies the base image to build on top of. Can be any standard image from Docker Hub or GHCR.

```dockerfile
# Start from Alpine Linux
base alpine:3.20

# Start a multi-stage builder
base golang:1.22 as builder
```

---

### 2. `workdir <path>`
Sets the current working directory for subsequent `copy` and `exec` commands. The folder is automatically created if it does not exist.

```dockerfile
workdir /app
```

---

### 3. `copy [--from=<stage>] <source> <destination>`
Copies files or folders from your computer into the container rootfs.

```dockerfile
# Copy local package files
copy package.json package-lock.json ./

# Copy everything from current folder
copy . .

# Copy built files from an earlier build stage
copy --from=builder /app/dist /usr/share/nginx/html
```

---

### 4. `exec <command> [args...]`
Runs a command to install dependencies, compile code, or prepare files during the image build.

```dockerfile
exec npm install
exec apk add --no-cache curl
exec python -m pip install -r requirements.txt
```

---

### 5. `env KEY=VALUE ...`
Sets one or more environment variables inside the container.

```dockerfile
env NODE_ENV=production PORT=3000
```

---

### 6. `expose <port>`
Declares which network port the application listens on.

```dockerfile
expose 3000
expose 80 443
```

---

### 7. `start <command> [args...]`
Specifies what command to run when the container starts up.

```dockerfile
start node index.js
start python app.py
start /usr/local/bin/server
```

---

### 8. `secure rootless=<bool> readonly=<bool> seccomp=<default|strict>`
*(Optional)* Declares the security profile for your image. By default, Sentra is already rootless and secure.

```dockerfile
secure rootless=true readonly=true seccomp=default
```

---

## Complete Multi-Stage Example

Multi-stage builds allow you to use heavy tools (Node compilers, Go compilers, build tools) to build your app, and then copy only the lightweight compiled output into a tiny, secure production image:

```dockerfile
# ================= Stage 1: Build =================
base node:20-alpine as builder
workdir /app

# Cache dependencies
copy package.json package-lock.json ./
exec npm install

# Copy source and build
copy . .
exec npm run build

# ================= Stage 2: Production ============
base nginx:alpine

# Copy ONLY the static build output from Stage 1
copy --from=builder /app/dist /usr/share/nginx/html

expose 80
start nginx -g "daemon off;"
```

---

## Validating Your Sentrafile

Before building, you can verify that your `Sentrafile` has no syntax errors using:

```sh
sentra config Sentrafile
```

To just check for a pass/fail confirmation:

```sh
sentra config -q Sentrafile
# Prints: ok
```
