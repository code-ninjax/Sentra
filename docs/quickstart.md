# Quickstart Guide

Get up and running with Sentra in under 60 seconds. No background daemon, no complicated setup, no background services eating your RAM.

---

## 1. Install Sentra (10 Seconds)

Sentra is a single, static binary (~8 MB). Run this in your terminal:

```sh
curl -fsSL https://trysentra.tech/install.sh | sh
```

Verify it's working:

```sh
sentra --version
```

> **Why this is different from Docker:** You do **not** need to start a service (`sudo systemctl start docker` or open Docker Desktop). Sentra runs directly as a command and exits when finished.

---

## 2. Run Any Existing Image

Sentra natively speaks OCI. It can pull and run any existing image from Docker Hub:

```sh
# Run a quick command and exit
sentra run alpine echo "Hello from Sentra!"

# Start an Nginx web server on port 8080
sentra run -d -p 8080:80 --name my-web nginx:alpine
```

Open **`http://localhost:8080`** in your browser. You're running a container!

---

## 3. Containerize Your Own Project

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

## 4. Build & Run Your App

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

## 5. The 5 Everyday Commands You Need

You only need five commands for 99% of your daily workflow:

| What you want to do | The Command |
|---|---|
| **Build your app** | `sentra build -t <app-name> .` |
| **Run your app on a port** | `sentra run -d -p 8080:80 --name <container-name> -t <app-name>` |
| **See running containers** | `sentra ps` |
| **View live logs** | `sentra logs -f <container-name>` |
| **Stop a container** | `sentra stop <container-name>` |

### Additional helpful commands:
- **Run a shell inside a container:** `sentra exec -it <container-name> -- /bin/sh`
- **List your images:** `sentra images`
- **Remove a stopped container:** `sentra rm <container-name>`

---

## 6. Where and Why to Use Sentra (Real Use Cases)

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
