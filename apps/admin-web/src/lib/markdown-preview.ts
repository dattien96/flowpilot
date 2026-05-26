function escapeHtml(value: string) {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

function renderInlineMarkdown(value: string) {
  return escapeHtml(value)
    .replace(/`([^`]+)`/g, "<code>$1</code>")
    .replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>")
    .replace(/\*(?!\s)([^*\n]+?)\*(?!\w)/g, "<em>$1</em>");
}

function renderMarkdownBody(markdown: string) {
  const lines = markdown.replace(/\r\n/g, "\n").split("\n");
  const blocks: string[] = [];
  let paragraphLines: string[] = [];
  let listItems: string[] = [];
  let quoteLines: string[] = [];
  let codeLines: string[] = [];
  let inCodeBlock = false;
  let codeLanguage = "";

  const flushParagraph = () => {
    if (paragraphLines.length === 0) {
      return;
    }
    blocks.push(`<p>${paragraphLines.map(renderInlineMarkdown).join(" ")}</p>`);
    paragraphLines = [];
  };

  const flushList = () => {
    if (listItems.length === 0) {
      return;
    }
    blocks.push(`<ul>${listItems.map((item) => `<li>${renderInlineMarkdown(item)}</li>`).join("")}</ul>`);
    listItems = [];
  };

  const flushQuote = () => {
    if (quoteLines.length === 0) {
      return;
    }
    blocks.push(
      `<blockquote>${quoteLines.map((line) => renderInlineMarkdown(line)).join("<br />")}</blockquote>`,
    );
    quoteLines = [];
  };

  const flushTextBlocks = () => {
    flushParagraph();
    flushList();
    flushQuote();
  };

  for (const rawLine of lines) {
    const line = rawLine.trimEnd();
    const codeFenceMatch = line.match(/^```(\w+)?\s*$/);

    if (codeFenceMatch) {
      if (inCodeBlock) {
        blocks.push(
          `<pre><code${codeLanguage ? ` class="language-${escapeHtml(codeLanguage)}"` : ""}>${escapeHtml(codeLines.join("\n"))}</code></pre>`,
        );
        codeLines = [];
        inCodeBlock = false;
        codeLanguage = "";
      } else {
        flushTextBlocks();
        inCodeBlock = true;
        codeLanguage = codeFenceMatch[1] ?? "";
      }
      continue;
    }

    if (inCodeBlock) {
      codeLines.push(rawLine);
      continue;
    }

    if (!line.trim()) {
      flushTextBlocks();
      continue;
    }

    const headingMatch = line.match(/^(#{1,6})\s+(.*)$/);
    if (headingMatch) {
      flushTextBlocks();
      const level = headingMatch[1].length;
      blocks.push(`<h${level}>${renderInlineMarkdown(headingMatch[2])}</h${level}>`);
      continue;
    }

    if (/^(-{3,}|\*{3,}|_{3,})\s*$/.test(line.trim())) {
      flushTextBlocks();
      blocks.push("<hr />");
      continue;
    }

    const quoteMatch = line.match(/^>\s?(.*)$/);
    if (quoteMatch) {
      flushParagraph();
      flushList();
      quoteLines.push(quoteMatch[1]);
      continue;
    }

    const listMatch = line.match(/^\s*[-*+]\s+(.*)$/);
    if (listMatch) {
      flushParagraph();
      flushQuote();
      listItems.push(listMatch[1]);
      continue;
    }

    flushList();
    flushQuote();
    paragraphLines.push(line.trim());
  }

  flushTextBlocks();

  if (inCodeBlock) {
    blocks.push(`<pre><code>${escapeHtml(codeLines.join("\n"))}</code></pre>`);
  }

  return blocks.join("\n");
}

export function renderMarkdownPreviewHtml(markdown: string, title = "Markdown Preview") {
  const body = renderMarkdownBody(markdown);

  return `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>${escapeHtml(title)}</title>
    <style>
      :root {
        color-scheme: dark;
        --bg: #0f1411;
        --panel: #141b17;
        --panel-strong: #1a241e;
        --border: rgba(120, 168, 136, 0.22);
        --text: #edf3ef;
        --muted: rgba(237, 243, 239, 0.68);
        --accent: #59a67b;
        --code-bg: rgba(10, 14, 12, 0.85);
      }

      * { box-sizing: border-box; }
      html, body { margin: 0; min-height: 100%; background:
        radial-gradient(circle at top, rgba(89, 166, 123, 0.14), transparent 32%),
        linear-gradient(180deg, #111813 0%, var(--bg) 100%);
        color: var(--text);
        font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      }
      body { padding: 32px 18px 48px; }
      .shell {
        max-width: 960px;
        margin: 0 auto;
        border: 1px solid var(--border);
        border-radius: 28px;
        background: rgba(20, 27, 23, 0.82);
        box-shadow: 0 24px 80px rgba(0, 0, 0, 0.32);
        overflow: hidden;
      }
      .header {
        padding: 20px 24px;
        border-bottom: 1px solid var(--border);
        background: linear-gradient(180deg, rgba(89, 166, 123, 0.12), transparent);
      }
      .eyebrow {
        font-size: 11px;
        letter-spacing: 0.28em;
        text-transform: uppercase;
        color: var(--muted);
        font-weight: 700;
      }
      .title {
        margin: 10px 0 0;
        font-size: 28px;
        line-height: 1.1;
        letter-spacing: -0.03em;
      }
      .content {
        padding: 28px 24px 32px;
      }
      .markdown {
        max-width: 820px;
        margin: 0 auto;
        line-height: 1.7;
        font-size: 16px;
        overflow-wrap: anywhere;
      }
      .markdown h1,
      .markdown h2,
      .markdown h3,
      .markdown h4,
      .markdown h5,
      .markdown h6 {
        margin: 1.4em 0 0.5em;
        line-height: 1.2;
        letter-spacing: -0.03em;
      }
      .markdown h1 { font-size: 2.2rem; }
      .markdown h2 { font-size: 1.8rem; }
      .markdown h3 { font-size: 1.45rem; }
      .markdown h4 { font-size: 1.2rem; }
      .markdown p { margin: 0 0 1em; color: var(--text); }
      .markdown ul,
      .markdown ol {
        margin: 0 0 1em 1.3em;
        padding: 0;
      }
      .markdown li { margin: 0.35em 0; }
      .markdown blockquote {
        margin: 0 0 1em;
        padding: 0.9em 1.1em;
        border-left: 3px solid var(--accent);
        background: rgba(89, 166, 123, 0.08);
        border-radius: 0 16px 16px 0;
        color: var(--muted);
      }
      .markdown hr {
        border: 0;
        border-top: 1px solid var(--border);
        margin: 1.6em 0;
      }
      .markdown code {
        padding: 0.14em 0.35em;
        border-radius: 8px;
        background: rgba(89, 166, 123, 0.12);
        font-family: "SFMono-Regular", ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
        font-size: 0.93em;
      }
      .markdown pre {
        margin: 0 0 1em;
        padding: 1em 1.1em;
        overflow: auto;
        border-radius: 18px;
        border: 1px solid var(--border);
        background: var(--code-bg);
      }
      .markdown pre code {
        padding: 0;
        background: transparent;
        display: block;
        white-space: pre;
      }
      .markdown a { color: #92e0b2; }
    </style>
  </head>
  <body>
    <main class="shell">
      <header class="header">
        <div class="eyebrow">Markdown Preview</div>
        <h1 class="title">${escapeHtml(title)}</h1>
      </header>
      <section class="content">
        <article class="markdown">
          ${body || "<p><em>No markdown content available.</em></p>"}
        </article>
      </section>
    </main>
  </body>
</html>`;
}

export function openMarkdownPreviewInNewTab(markdown: string, title = "Markdown Preview") {
  const html = renderMarkdownPreviewHtml(markdown, title);
  const tab = window.open("", "_blank");

  if (!tab) {
    return false;
  }

  tab.opener = null;
  tab.document.open();
  tab.document.write(html);
  tab.document.close();
  return true;
}
