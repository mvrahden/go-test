// The panel's entry points, for the modules that wire it: its host and the
// reading of the spec document.
export * from "./panel/specData.js";
export {
  SpecViewPanel,
  parseStoredState,
  type Scope,
  type SpecViewActions,
} from "./panel/host.js";
