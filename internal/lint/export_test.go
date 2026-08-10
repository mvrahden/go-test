package lint

var ExportParseNolint = parseNolint

func ExportSetDisableNolint(v bool) { cfg.disableNolint = v }

func ExportSetSkip(r Rule, v bool) { *cfg.skip[r] = v }
