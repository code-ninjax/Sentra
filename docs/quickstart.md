# Quickstart Guide

Get up and running with Sentra in under 60 seconds. No background daemon, no complicated setup, no background services eating your RAM.

---

## 1. Install Sentra (60 Seconds)

One command. Copy it, paste it, done:

```sh
curl -fsSL https://trysentra.tech/install.sh | sh
```

Check it worked (you should see this back):

```sh
sentra --version
# sentra version v0.1.0 (linux/amd64)
```

That's it. No service to start, no daemon running in the background.

**On Windows?** Install WSL2 first — run `wsl --install` in PowerShell and restart. Then open Ubuntu and run the same command above. Sentra lives inside WSL2. To also use it straight from PowerShell, grab `sentra.exe` from the [Releases page](https://github.com/code-ninjax/Sentra/releases), put it on your PATH, and `sentra ps` will work there too (it just forwards into WSL).

**Missing `runc`?** Most systems already have it. If anything complains, run `sudo apt install -y runc` and retry.

> **Why this is different from Docker:** You do **not** need to start a service (`sudo systemctl start docker` or open Docker Desktop). Sentra runs directly as a command and exits when finished.

---

## 2. Containerize a Project, Start to Finish

Here is the whole flow on a real project. Make a folder with two files — `index.html` can be anything:

```sh
mkdir demo && cd demo
```

`Sentrafile` (this is the entire build recipe — plain words, no shell):

```dockerfile
base alpine:3.20
workdir /app
copy index.html /app/index.html
start cat /app/index.html
```

Check the plan before you build (shows every step Sentra will run):

```sh
sentra config Sentrafile
# stages  1
# secure  rootless=true readonly=true seccomp=default
#
# image    (final)  base alpine:3.20
#   steps  3
#   2      workdir  /app
#   3      copy     index.html -> /app/index.html
#   4      start    cat /app/index.html
```

Build it (downloads alpine once, ~3 seconds after that):

```sh
sentra build -t demo .
# note: per-step layer caching unavailable (overlayfs mount: operation not permitted)
#       building without cache; run as root for fast rebuilds
# base alpine:3.20
# pulling index.docker.io/library/alpine:3.20@sha256:c64c...
#   layer 1/1 cached (25f1d6b1951a)
# rootfs ready: /home/you/.sentra/rootfs/library-alpine-3.20
# executing 3 steps sequentially
# [1] copy    index.html -> /app/index.html
#
# built demo in 3.1s (0/1 layers cached)
# rootfs: /home/you/.sentra/built/demo-rootfs
```

*(The cache note is normal for rootless builds — overlay mounts need privilege. It still builds fine.)*

Run it — output lands straight in your terminal:

```sh
sentra run -t demo
# sentra: rootless, read-only rootfs, seccomp=default
# sentra: container networking needs root; this container gets loopback only
#         (run with sudo for outbound networking and -p)
# <!doctype html><html><body><h1>hello from sentra</h1></body></html>
```

The `sentra:` lines go to stderr — that is posture info (what security you got), not errors. Your container's own output is the last line.

Run it in the background and work the lifecycle (`-t demo --` overrides the image command with your own):

```sh
sentra run -d --name demo-sleep -t demo -- /bin/sleep 300
# sentra: rootless, read-only rootfs, seccomp=default
# sentra: container networking needs root; ...
# demo-sleep
```

```sh
sentra ps
# ID              STATUS   CMD                CREATED
# demo-sleep      running  /bin/sleep 300     0001-01-01 00:00
```

*(Old exited containers pile up in this list too — trimmed here. The `0001-01-01` dates are a known display wart.)*

```sh
sentra logs demo-sleep
# (no output — sleep prints nothing; a server would show its logs here)
```

```sh
sentra exec demo-sleep /bin/echo hi-from-inside
# hi-from-inside
```

> No `--` before the command with `exec` — unlike `run`, `exec` takes the command directly. With `--` you get `exec: "--": executable file not found`.

```sh
sentra stop demo-sleep
# (silent — sends SIGTERM; a PID-1 sleeper may ignore it, which is normal Unix, not a bug)

sentra rm demo-sleep
# sentra: container "demo-sleep" is running; use -f to force

sentra rm -f demo-sleep
# demo-sleep
```

```sh
sentra images
# TAG           ROOTFS                                  ENTRYPOINT
# demo          /home/you/.sentra/built/demo-rootfs     cat /app/index.html
```

That is the full loop: write 4 lines, build, run, inspect, clean up.

---

## 3. Run Any Existing Image

Sentra natively speaks OCI. It can pull and run any existing image from Docker Hub:

```sh
sentra pull alpine:3.20
# pulling index.docker.io/library/alpine:3.20@sha256:c64c...
#   layer 1/1 cached (25f1d6b1951a)
# rootfs ready: /home/you/.sentra/rootfs/library-alpine-3.20
```

```sh
# Run a quick command and exit
sentra run --rootfs ~/.sentra/rootfs/library-alpine-3.20 -- /bin/echo "Hello from Sentra!"
# sentra: rootless, read-only rootfs, seccomp=default
# sentra: container networking needs root; this container gets loopback only
#         (run with sudo for outbound networking and -p)
# Hello from Sentra!
```

`--rootfs` points at the unpacked image `pull` made for you. (After `sentra build`, you can use the shorter `-t <tag>` form from section 2 instead.)

---

## 4. Containerize Your Own Project

To package your app, create a file named `Sentrafile` in your project root. A `Sentrafile` is just a list of instructions on how to build and run your code.

### Recipe A: Frontend App (React, Vite, Vue, Next static)

Build your production assets and serve them with a fast Nginx web server:

```dockerfile
# Step 1: Install packages and build static files
base node:20-alpine as builder
workdir /app
copy package.json package-lock.json ./
exec npm install
copy . .
exec npm run build

# Step 2: Serve the build folder with Nginx
base nginx:alpine
copy --from=builder /app/dist /usr/share/nginx/html
expose 80
start nginx -g "daemon off;"
```

*(If your project outputs to `build` instead of `dist`, change `/app/dist` to `/app/build`)*.

---

### Recipe B: Node.js / Express Backend API

```dockerfile
base node:20-alpine
workdir /app

# Install dependencies first (cached for fast builds)
copy package.json package-lock.json ./
exec npm install --production

# Copy application source code
copy . .

expose 3000
start node server.js
```

---

### Recipe C: Python / FastAPI / Flask API

```dockerfile
base python:3.12-slim
workdir /app

# Install requirements
copy requirements.txt .
exec pip install --no-cache-dir -r requirements.txt

# Copy application source code
copy . .

expose 8000
start python main.py
```

---

## 5. Build & Run Your App

Once your `Sentrafile` is in your project directory:

### Step 1: Build the image

```sh
sentra build -t my-app .
```

*What this does:* Sentra reads your `Sentrafile`, downloads layers in parallel, executes your build steps, and tags the output as `my-app`.

### Step 2: Run the container

```sh
sentra run -d -p 8080:80 --name my-running-app -t my-app
```

*In plain English:*
- `-d` = Run in the background (detached), so your terminal stays free.
- `-p 8080:80` = Maps port `8080` on your machine to port `80` inside the container.
- `--name my-running-app` = Gives the container a nickname so you can manage it.
- `-t my-app` = Tells Sentra which built image to run.

Open **`http://localhost:8080`** in your browser to view your live app!

---

## 6. The 5 Everyday Commands You Need

You only need five commands for 99% of your daily workflow:

| What you want to do | The Command |
|---|---|
| **Build your app** | `sentra build -t <app-name> .` |
| **Run your app on a port** | `sentra run -d -p 8080:80 --name <container-name> -t <app-name>` |
| **See running containers** | `sentra ps` |
| **View live logs** | `sentra logs -f <container-name>` |
| **Stop a container** | `sentra stop <container-name>` |

### Additional helpful commands:
- **Run a shell command inside a container:** `sentra exec <container-name> /bin/echo hello`
- **List your images:** `sentra images`
- **Remove a stopped container:** `sentra rm <container-name>`

---

## 7. Where and Why to Use Sentra (Real Use Cases)

Sentra was built to replace heavyweight container engines in everyday development:

### 1. Everyday Local Web & App Development
- **No background battery drain:** Docker Desktop consumes 2–4 GB of RAM and drains laptop battery even when you aren't running anything. Sentra has no background daemon; it only uses CPU/RAM when your container is actually executing.
- **Instant startup:** You don't wait for a virtual machine or daemon to boot up.

### 2. Fast CI/CD Pipelines (GitHub Actions, GitLab CI)
- **Zero Docker-in-Docker headaches:** In CI runners, Docker requires privileged root access or complex DinD setups. Sentra runs as an unprivileged static binary, saving setup time and improving pipeline security.
- **Parallel layer caching:** Sentra builds multi-stage pipelines with a parallel DAG scheduler.

### 3. Edge Computing & Low-Spec Cloud VPS
- If you deploy to a \$5/mo VPS (1GB RAM on DigitalOcean, Hetzner, Linode) or a Raspberry Pi, Docker's daemon overhead takes up a noticeable chunk of system memory.
- Sentra uses **0 MB of idle memory**. When your containers aren't running, Sentra uses zero resources.

### 4. Microservices & Isolated Test Suites
- Spin up ephemeral Redis, PostgreSQL, or mock services during unit and integration test runs, run the test, and tear them down in milliseconds without socket pollution.

---

## Next Steps

- Check out [Sentrafile syntax](/#/docs/sentrafile) for the complete list of directives and build options.
- Read [Sentra vs Docker vs Podman](/#/docs/comparison) to see how Sentra compares under the hood.
