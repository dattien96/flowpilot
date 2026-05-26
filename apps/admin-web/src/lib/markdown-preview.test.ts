import { afterEach, describe, expect, it, vi } from "vitest";

import { openMarkdownPreviewInNewTab, renderMarkdownPreviewHtml } from "./markdown-preview";

afterEach(() => {
  vi.restoreAllMocks();
});

describe("renderMarkdownPreviewHtml", () => {
  it("renders common markdown blocks into preview html", () => {
    const html = renderMarkdownPreviewHtml(
      "# Title\n\n- First item\n- Second item\n\n```ts\nconst x = 1;\n```",
      "Example",
    );

    expect(html).toContain("<h1>Title</h1>");
    expect(html).toContain("<ul><li>First item</li><li>Second item</li></ul>");
    expect(html).toContain("<code class=\"language-ts\">const x = 1;</code>");
    expect(html).toContain("<title>Example</title>");
  });

  it("escapes raw html in markdown content", () => {
    const html = renderMarkdownPreviewHtml("<script>alert('x')</script>", "Safe");

    expect(html).toContain("&lt;script&gt;alert(&#39;x&#39;)&lt;/script&gt;");
    expect(html).not.toContain("<script>alert('x')</script>");
  });

  it("opens a blank tab and writes the rendered preview html into it", () => {
    const write = vi.fn();
    const open = vi.fn();
    const close = vi.fn();
    const tab = {
      opener: {},
      document: {
        open,
        write,
        close,
      },
    } as unknown as Window;

    vi.spyOn(window, "open").mockReturnValue(tab);

    const result = openMarkdownPreviewInNewTab("# Title", "Preview");

    expect(result).toBe(true);
    expect(window.open).toHaveBeenCalledWith("", "_blank");
    expect(open).toHaveBeenCalled();
    expect(write).toHaveBeenCalledWith(expect.stringContaining("<h1>Title</h1>"));
    expect(close).toHaveBeenCalled();
  });
});
