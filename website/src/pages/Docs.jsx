import React, { useEffect, useState, useMemo } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import sentrafileMd from "../../../docs/sentrafile.md?raw";
import quickstartMd from "../../../docs/quickstart.md?raw";
import architectureMd from "../../../docs/architecture.md?raw";
import comparisonMd from "../../../docs/comparison.md?raw";

const PAGES = [
  {
    id: "quickstart",
    label: "Quickstart",
    description: "Install, pull, build and run your first container",
    content: quickstartMd,
  },
  {
    id: "sentrafile",
    label: "Sentrafile syntax",
    description: "Directives, grammar, and multi-stage build rules",
    content: sentrafileMd,
  },
  {
    id: "architecture",
    label: "Architecture",
    description: "Namespaces, rootfs layers, caching DAG and runtime",
    content: architectureMd,
  },
  {
    id: "comparison",
    label: "vs Docker / Podman",
    description: "Feature matrix and architectural tradeoffs",
    content: comparisonMd,
  },
];

function extractHashSlug(hash) {
  const clean = hash.replace(/^#\/?docs\/?/, "").trim();
  return clean || "quickstart";
}

// OS picker for the Quickstart page: Windows / Linux switch with
// copyable install commands. One command per step, plain words.
const INSTALL_TABS = {
  linux: {
    label: "Linux",
    steps: [
      { text: "Paste this in your terminal:", cmd: "curl -fsSL https://trysentra.tech/install.sh | sh" },
      { text: "Check it worked:", cmd: "sentra --version" },
    ],
    note: "Missing runc? Most systems have it. If not: sudo apt install -y runc, then retry.",
  },
  windows: {
    label: "Windows",
    steps: [
      { text: "In PowerShell, install WSL2 (restart after):", cmd: "wsl --install" },
      { text: "Open Ubuntu, then paste the same installer:", cmd: "curl -fsSL https://trysentra.tech/install.sh | sh" },
      { text: "Check it worked (in Ubuntu):", cmd: "sentra --version" },
      { text: "Want it in PowerShell too? Download the forwarder:", cmd: "Invoke-WebRequest https://github.com/code-ninjax/Sentra/releases/latest/download/sentra.exe -OutFile $HOME\\bin\\sentra.exe" },
    ],
    note: "Put sentra.exe somewhere on your PATH — then sentra works from PowerShell by forwarding into WSL2.",
  },
};

function OsToggle() {
  const [os, setOs] = useState("linux");
  const [copied, setCopied] = useState(-1);
  const copy = (i, text) => {
    try {
      navigator.clipboard.writeText(text);
    } catch {
      /* clipboard unavailable */
    }
    setCopied(i);
    setTimeout(() => setCopied(-1), 1500);
  };
  const tab = INSTALL_TABS[os];
  return (
    <div className="docs-os-toggle">
      <div className="docs-os-tabs" role="tablist" aria-label="Choose your system">
        {Object.entries(INSTALL_TABS).map(([id, t]) => (
          <button
            key={id}
            role="tab"
            aria-selected={os === id}
            className={"docs-os-tab" + (os === id ? " active" : "")}
            onClick={() => setOs(id)}
          >
            {t.label}
          </button>
        ))}
      </div>
      <ol className="docs-os-steps">
        {tab.steps.map((s, i) => (
          <li key={i}>
            <p>{s.text}</p>
            <div className="docs-os-cmd">
              <code className="mono">{s.cmd}</code>
              <button type="button" className="docs-code-copy" onClick={() => copy(i, s.cmd)}>
                {copied === i ? "Copied" : "Copy"}
              </button>
            </div>
          </li>
        ))}
      </ol>
      <p className="docs-os-note">{tab.note}</p>
    </div>
  );
}

function CodePre({ children, ...props }) {  const [copied, setCopied] = useState(false);

  // Extract raw text content from children (which is a <code> tag)
  const extractText = (node) => {
    if (typeof node === "string") return node;
    if (Array.isArray(node)) return node.map(extractText).join("");
    if (node && node.props && node.props.children) {
      return extractText(node.props.children);
    }
    return "";
  };

  const handleCopy = () => {
    const raw = extractText(children).trim();
    if (!raw) return;
    navigator.clipboard.writeText(raw);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="docs-code-container">
      <div className="docs-code-bar">
        <span className="docs-code-dot" />
        <span className="docs-code-label">SHELL</span>
        <button
          type="button"
          className="docs-code-copy"
          onClick={handleCopy}
          aria-label="Copy snippet"
        >
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
      <pre {...props}>{children}</pre>
    </div>
  );
}

export default function Docs() {
  const [active, setActive] = useState(() => {
    const slug = extractHashSlug(window.location.hash);
    return PAGES.some((p) => p.id === slug) ? slug : "quickstart";
  });

  useEffect(() => {
    const onHashChange = () => {
      const slug = extractHashSlug(window.location.hash);
      if (slug && PAGES.some((p) => p.id === slug)) {
        setActive(slug);
      }
    };
    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  const select = (id) => {
    setActive(id);
    window.location.hash = "#/docs/" + id;
    window.scrollTo({ top: 0, behavior: "smooth" });
  };

  const currentIndex = PAGES.findIndex((p) => p.id === active);
  const current = currentIndex >= 0 ? PAGES[currentIndex] : PAGES[0];
  const prevPage = currentIndex > 0 ? PAGES[currentIndex - 1] : null;
  const nextPage = currentIndex < PAGES.length - 1 ? PAGES[currentIndex + 1] : null;

  const markdownComponents = useMemo(
    () => ({
      pre: CodePre,
      table: ({ children, ...props }) => (
        <div className="docs-table-wrap">
          <table {...props}>{children}</table>
        </div>
      ),
      a: ({ href, children, ...props }) => {
        const isExternal = href?.startsWith("http");
        return (
          <a
            href={href}
            target={isExternal ? "_blank" : undefined}
            rel={isExternal ? "noopener noreferrer" : undefined}
            {...props}
          >
            {children}
          </a>
        );
      },
    }),
    []
  );

  return (
    <div className="docs-layout">
      <div className="docs-container">
        {/* Mobile quick tabs */}
        <div className="docs-mobile-tabs" role="tablist" aria-label="Documentation sections">
          {PAGES.map((p) => (
            <button
              key={p.id}
              role="tab"
              aria-selected={active === p.id}
              className={"docs-tab-chip" + (active === p.id ? " active" : "")}
              onClick={() => select(p.id)}
            >
              {p.label}
            </button>
          ))}
        </div>

        {/* Sidebar */}
        <aside className="docs-sidebar" aria-label="Docs navigation">
          <div className="docs-sidebar-header">
            <span className="docs-sidebar-badge">SENTRA</span>
            <span className="docs-sidebar-title">DOCUMENTATION</span>
          </div>
          <nav className="docs-sidebar-nav">
            {PAGES.map((p) => (
              <button
                key={p.id}
                className={"docs-nav-link" + (active === p.id ? " active" : "")}
                onClick={() => select(p.id)}
              >
                <span className="docs-nav-indicator" />
                <div className="docs-nav-text">
                  <span className="docs-nav-name">{p.label}</span>
                  <span className="docs-nav-desc">{p.description}</span>
                </div>
              </button>
            ))}
          </nav>

          <div className="docs-sidebar-footer">
            <a
              href="/llms.txt"
              target="_blank"
              rel="noopener noreferrer"
              className="docs-sidebar-github"
              style={{ marginBottom: "8px" }}
            >
              LLM & Agent Context (llms.txt) ↗
            </a>
            <a
              href="https://github.com/code-ninjax/Sentra"
              target="_blank"
              rel="noopener noreferrer"
              className="docs-sidebar-github"
            >
              View repository on GitHub ↗
            </a>
          </div>
        </aside>

        {/* Main Content Area */}
        <main className="docs-content" id="docs-content">
          <div className="docs-top-bar">
            <div className="docs-breadcrumb">
              <a href="#/" className="docs-crumb-link">Home</a>
              <span className="docs-crumb-sep">/</span>
              <span className="docs-crumb-mid">Docs</span>
              <span className="docs-crumb-sep">/</span>
              <span className="docs-crumb-current">{current.label}</span>
            </div>
            <a
              href={`https://github.com/code-ninjax/Sentra/blob/main/docs/${current.id}.md`}
              target="_blank"
              rel="noopener noreferrer"
              className="docs-edit-btn"
            >
              Edit on GitHub ↗
            </a>
          </div>

          <article className="docs-prose">
            {active === "quickstart" && <OsToggle />}
            <ReactMarkdown
              remarkPlugins={[remarkGfm]}
              components={markdownComponents}
            >
              {current.content}
            </ReactMarkdown>
          </article>

          {/* Bottom Pagination */}
          <nav className="docs-pagination" aria-label="Docs pagination">
            {prevPage ? (
              <button
                type="button"
                className="docs-pager-btn prev"
                onClick={() => select(prevPage.id)}
              >
                <span className="pager-arrow">←</span>
                <span className="pager-meta">
                  <span className="pager-direction">Previous</span>
                  <span className="pager-title">{prevPage.label}</span>
                </span>
              </button>
            ) : (
              <div />
            )}

            {nextPage ? (
              <button
                type="button"
                className="docs-pager-btn next"
                onClick={() => select(nextPage.id)}
              >
                <span className="pager-meta">
                  <span className="pager-direction">Next</span>
                  <span className="pager-title">{nextPage.label}</span>
                </span>
                <span className="pager-arrow">→</span>
              </button>
            ) : (
              <div />
            )}
          </nav>
        </main>
      </div>
    </div>
  );
}
