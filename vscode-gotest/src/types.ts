export interface DiscoverOutput {
  packages: DiscoverPackage[];
}

export interface DiscoverPackage {
  importPath: string;
  dir: string;
  suites: DiscoverSuite[];
}

export interface DiscoverSuite {
  name: string;
  parallel: boolean;
  focused: boolean;
  excluded: boolean;
  file: string;
  line: number;
  col: number;
  lifecycle: string[];
  fixtures: string[];
  methods: DiscoverMethod[];
}

export interface DiscoverMethod {
  name: string;
  parallel: boolean;
  focused: boolean;
  excluded: boolean;
  file: string;
  line: number;
  col: number;
}

export interface OverlayOutput {
  overlayFile: string;
  dir: string;
}
