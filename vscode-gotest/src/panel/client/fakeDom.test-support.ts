// A stand-in for the few DOM calls the webview's app makes: elements by id,
// created elements, attributes, data attributes and closest() by [data-*].

export class FakeEl {
  children: FakeEl[] = [];
  parent: FakeEl | undefined;
  attrs = new Map<string, string>();
  dataset: Record<string, string> = {};
  className = "";
  hidden = false;
  disabled = false;
  open = false;
  value = "";
  scrollTop = 0;
  textContent: string | null = "";
  private html = "";

  constructor(
    readonly tagName: string,
    readonly id = "",
  ) {}

  get innerHTML(): string {
    return this.html;
  }

  set innerHTML(v: string) {
    this.html = v;
    for (const c of this.children) c.parent = undefined;
    this.children = [];
  }

  appendChild(c: FakeEl): FakeEl {
    c.remove();
    c.parent = this;
    this.children.push(c);
    return c;
  }

  remove(): void {
    if (!this.parent) return;
    this.parent.children = this.parent.children.filter((x) => x !== this);
    this.parent = undefined;
  }

  setAttribute(n: string, v: string): void {
    this.attrs.set(n, v);
  }

  getAttribute(n: string): string | null {
    return this.attrs.get(n) ?? null;
  }

  removeAttribute(n: string): void {
    this.attrs.delete(n);
  }

  // closest knows "[data-x]", "[role=x]", "#id" and ".class".
  closest(sel: string): FakeEl | null {
    for (let e: FakeEl | undefined = this; e; e = e.parent) {
      const data = /^\[data-([\w-]+)\]$/.exec(sel);
      if (data) {
        const key = data[1].replace(/-(\w)/g, (_, c: string) =>
          c.toUpperCase(),
        );
        if (e.dataset[key] !== undefined) return e;
      } else if (sel.startsWith("#") && e.id === sel.slice(1)) return e;
      else if (
        sel.startsWith(".") &&
        e.className.split(" ").includes(sel.slice(1))
      )
        return e;
      else {
        const role = /^\[role=(\w+)\]$/.exec(sel);
        if (role && e.getAttribute("role") === role[1]) return e;
      }
    }
    return null;
  }

  // querySelector knows [data-key="…"] in what was drawn here.
  querySelector(sel: string): FakeEl | null {
    const m = /^\[data-key="(.*)"\]$/.exec(sel);
    if (!m || !this.html.includes(`data-key="${m[1]}"`)) return null;
    const el = new FakeEl("div");
    el.dataset.key = m[1];
    if (m[1].startsWith("p:")) el.dataset.pkg = m[1].slice(2);
    el.focus = () => {
      FakeEl.focused = m[1];
    };
    return el;
  }

  // querySelectorAll knows [data-lazy] in what was drawn here.
  querySelectorAll(sel = ""): FakeEl[] {
    if (sel !== "[data-lazy]") return [];
    return [...this.html.matchAll(/data-lazy="([^"]+)"/g)].map((m) => {
      const el = new FakeEl("div");
      el.dataset.lazy = m[1];
      return el;
    });
  }

  // The key of the row last focused through querySelector.
  static focused: string | undefined;

  matches(): boolean {
    return false;
  }

  focus(): void {}

  // All text this element and its children hold, markup included.
  allHtml(): string {
    return (
      (this.textContent ?? "") +
      this.html +
      this.children.map((c) => c.allHtml()).join("")
    );
  }
}

export const SHELL_IDS = [
  "tab-btn-now",
  "tab-btn-change",
  "tab-btn-runs",
  "tab-now",
  "tab-change",
  "tab-runs",
  "badge-now",
  "badge-change",
  "primary",
  "updating",
  "menu",
  "more",
  "now-empty",
  "now-ctx",
  "now-chips",
  "now-attention",
  "now-all",
  "now-total",
  "now-groups",
  "now-packages",
  "now-found",
  "ctxmenu",
  "toast",
  "search",
  "change-ctx",
  "change-body",
  "runs-running",
  "runs-ctx",
  "runs-body",
];

export function fakeDom() {
  FakeEl.focused = undefined;
  const byId = new Map(SHELL_IDS.map((id) => [id, new FakeEl("div", id)]));
  const listeners = new Map<string, ((e: unknown) => void)[]>();
  const on = (type: string, fn: (e: unknown) => void) =>
    listeners.set(type, [...(listeners.get(type) ?? []), fn]);
  const document = {
    getElementById: (id: string) => byId.get(id) ?? null,
    createElement: (tag: string) => new FakeEl(tag),
    addEventListener: on,
  };
  const timers: (() => void)[] = [];
  const window = {
    addEventListener: on,
    setTimeout: (fn: () => void) => {
      timers.push(fn);
      return timers.length;
    },
    clearTimeout: () => {},
  };
  const posted: unknown[] = [];
  let state: unknown = null;
  const vscode = {
    postMessage: (m: unknown) => posted.push(m),
    getState: () => state,
    setState: (s: unknown) => {
      state = s;
    },
  };
  return {
    document,
    window,
    vscode,
    posted,
    el: (id: string) => byId.get(id)!,
    state: () => state as Record<string, unknown> | null,
    setState: (s: unknown) => (state = s),
    // send delivers a host message as the webview would receive it.
    get focused() {
      return FakeEl.focused;
    },
    // fire delivers a DOM event to the document's listeners.
    fire: (type: string, e: unknown) =>
      (listeners.get(type) ?? []).forEach((fn) => fn(e)),
    send: (data: unknown) =>
      (listeners.get("message") ?? []).forEach((fn) => fn({ data })),
    flushTimers: () => timers.splice(0).forEach((fn) => fn()),
  };
}
