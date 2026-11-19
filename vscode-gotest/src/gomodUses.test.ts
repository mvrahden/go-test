import { describe, it, expect } from "vitest";
import { moduleUsesGotest } from "./gomod.js";

const CLI = "github.com/mvrahden/go-test/cmd/gotest";
const mod = (body: string) =>
  `module example.com/app\n\ngo 1.26.0\n\n${body}\n`;

describe("moduleUsesGotest", () => {
  it.each([
    [
      "a direct single-line require",
      mod("require github.com/mvrahden/go-test v1.38.0"),
      true,
    ],
    [
      "a direct require in a block",
      mod("require (\n\tgithub.com/mvrahden/go-test v1.38.0\n)"),
      true,
    ],
    [
      "a tool directive beside an indirect require",
      mod(
        `tool ${CLI}\n\nrequire github.com/mvrahden/go-test v1.38.0 // indirect`,
      ),
      true,
    ],
    [
      "a replace directive",
      mod("replace github.com/mvrahden/go-test => ../gotest"),
      true,
    ],
    [
      "the gotest module itself",
      "module github.com/mvrahden/go-test\n\ngo 1.26.0\n",
      true,
    ],
    [
      "an indirect require alone",
      mod("require github.com/mvrahden/go-test v1.38.0 // indirect"),
      false,
    ],
    [
      "an indirect require in a block",
      mod("require (\n\tgithub.com/mvrahden/go-test v1.38.0 // indirect\n)"),
      false,
    ],
    [
      "a commented-out require",
      mod("// require github.com/mvrahden/go-test v1.38.0"),
      false,
    ],
    [
      "another module with the same prefix",
      mod("require github.com/mvrahden/go-test-extras v1.0.0"),
      false,
    ],
    ["no mention", mod("require golang.org/x/sys v0.49.0"), false],
  ])("%s", (_name, goMod, want) => {
    expect(moduleUsesGotest(goMod, CLI)).toBe(want);
  });
});
