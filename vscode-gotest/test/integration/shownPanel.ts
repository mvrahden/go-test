// What the panel's webview shows, read the way the webview reads it: the
// host's posted messages replayed through the webview's own app at the
// Behaviors depth, every package's rows asked for, into a stand-in DOM.
import { panelApp } from "../../src/panel/client/app.js";
import { clientHelpers } from "../../src/panel/client/helpers.js";
import { renderChange } from "../../src/panel/client/renderChange.js";
import { renderNow } from "../../src/panel/client/renderNow.js";
import { renderRuns } from "../../src/panel/client/renderRuns.js";
import {
  fakeDom,
  FakeEl,
} from "../../src/panel/client/fakeDom.test-support.js";
import type { HostMessage, Tab } from "../../src/panel/protocol.js";

interface Webview {
  posted: unknown[];
  post?: (msg: unknown) => void;
}

const REGIONS = [
  "now-ctx",
  "now-chips",
  "now-attention",
  "now-total",
  "now-groups",
  "now-packages",
  "now-found",
  "now-empty",
  "change-ctx",
  "change-body",
  "runs-running",
  "runs-ctx",
  "runs-body",
];

// settle lets the host's coalesced posts go.
export const settle = () => new Promise((r) => setTimeout(r, 350));

// shownHtml is the markup of every tab after the host's messages, with each
// package of the newest Now summary opened.
export async function shownHtml(
  webview: Webview,
  tab: Tab = "now",
): Promise<string> {
  await settle();
  const nows = webview.posted.filter(
    (m) => (m as HostMessage).type === "now",
  ) as Extract<HostMessage, { type: "now" }>[];
  const pkgs =
    nows.at(-1)?.model.folders.flatMap((f) => f.rows.map((r) => r.pkg)) ?? [];
  for (const pkg of pkgs) webview.post?.({ type: "openPackage", pkg });
  const H = clientHelpers();
  const dom = fakeDom();
  dom.setState({ tab, depth: "behaviors" });
  const app = panelApp({
    vscode: dom.vscode,
    document: dom.document,
    window: dom.window,
    initial: { has: { change: true, runs: true } },
    NOW: renderNow(H),
    CHANGE: renderChange(H),
    RUNS: renderRuns(H),
  });
  for (const m of webview.posted) app.receive(m as HostMessage);
  for (const t of ["now", "change", "runs"] as Tab[])
    app.receive({ type: "tab", tab: t });
  return REGIONS.map((id) => dom.el(id).allHtml()).join("\n");
}

// lastNow is the newest Now summary the host posted.
export function lastNow(webview: Webview) {
  const nows = webview.posted.filter(
    (m) => (m as HostMessage).type === "now",
  ) as Extract<HostMessage, { type: "now" }>[];
  return nows.at(-1)?.model;
}
