export interface DiscoverWarning {
  importPath: string;
  file?: string;
  line?: number;
  col?: number;
  message: string;
}

export interface DiscoverOutput {
  packages: DiscoverPackage[];
  warnings?: DiscoverWarning[];
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
  guarded: boolean;
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

export interface PrepareOutput {
  overlayFile: string;
  dir: string;
  stateFile?: string;
}
