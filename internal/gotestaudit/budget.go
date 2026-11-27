package gotestaudit

import (
	"go/ast"
	"go/constant"
	"go/types"
	"time"

	"github.com/mvrahden/go-test/internal/gotestast"
)

// defaultBudget is what a zero duration or a missing SuiteConfig means.
const defaultBudget = 30 * time.Second

// presets are the SuiteConfig presets' durations.
var presets = map[string][2]time.Duration{
	"DefaultSuiteConfig":     {30 * time.Second, 30 * time.Second},
	"IntegrationSuiteConfig": {2 * time.Minute, 5 * time.Minute},
}

// readBudget reads a suite's Timeout and SetupTimeout from its SuiteConfig
// in the forms the generator accepts; anything else is unknown.
func readBudget(ts *gotestast.TestSuiteSpec) Budget {
	decl := ts.ConfigDecl()
	if decl == nil {
		return Budget{Timeout: defaultBudget.String(), SetupTimeout: defaultBudget.String()}
	}
	info := ts.Package().TypesInfo
	body, err := gotestast.ParseSuiteConfigBody(info, decl)
	if err != nil {
		return Budget{Timeout: BudgetUnknown, SetupTimeout: BudgetUnknown}
	}
	var timeout, setup string
	switch {
	case body.Literal != nil:
		timeout, setup = budgetString(0), budgetString(0)
		for _, kv := range body.Fields {
			key, _ := kv.Key.(*ast.Ident)
			if key == nil {
				continue
			}
			switch key.Name {
			case "Timeout":
				timeout = budgetValue(info, kv.Value)
			case "SetupTimeout":
				setup = budgetValue(info, kv.Value)
			}
		}
	default:
		timeout, setup = presetBudget(info, body.Base)
	}
	for _, assign := range body.Assigns {
		sel, _ := assign.Lhs[0].(*ast.SelectorExpr)
		if sel == nil {
			continue
		}
		switch sel.Sel.Name {
		case "Timeout":
			timeout = budgetValue(info, assign.Rhs[0])
		case "SetupTimeout":
			setup = budgetValue(info, assign.Rhs[0])
		}
	}
	return Budget{Timeout: timeout, SetupTimeout: setup}
}

// presetBudget is the budget of a preset call.
func presetBudget(info *types.Info, base ast.Expr) (timeout, setup string) {
	call, _ := base.(*ast.CallExpr)
	if call == nil {
		return BudgetUnknown, BudgetUnknown
	}
	var id *ast.Ident
	switch f := call.Fun.(type) {
	case *ast.SelectorExpr:
		id = f.Sel
	case *ast.Ident:
		id = f
	}
	if id == nil {
		return BudgetUnknown, BudgetUnknown
	}
	fn, _ := info.Uses[id].(*types.Func)
	d, ok := presets[id.Name]
	if fn == nil || fn.Pkg() == nil || fn.Pkg().Path() != gotestPath || !ok {
		return BudgetUnknown, BudgetUnknown
	}
	return budgetString(d[0]), budgetString(d[1])
}

// budgetValue is a duration expression's budget: a constant, or unknown.
func budgetValue(info *types.Info, expr ast.Expr) string {
	tv, ok := info.Types[expr]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.Int {
		return BudgetUnknown
	}
	n, exact := constant.Int64Val(tv.Value)
	if !exact {
		return BudgetUnknown
	}
	return budgetString(time.Duration(n))
}

// budgetString spells a duration as the record does: zero is the default.
func budgetString(d time.Duration) string {
	switch {
	case d < 0:
		return BudgetNone
	case d == 0:
		return defaultBudget.String()
	default:
		return d.String()
	}
}

// budgetLess reports whether budget a is shorter than b; ok is false when
// either is unknown.
func budgetLess(a, b string) (less, ok bool) {
	if a == BudgetUnknown || b == BudgetUnknown {
		return false, false
	}
	if a == b {
		return false, true
	}
	if a == BudgetNone {
		return false, true
	}
	if b == BudgetNone {
		return true, true
	}
	da, errA := time.ParseDuration(a)
	db, errB := time.ParseDuration(b)
	if errA != nil || errB != nil {
		return false, false
	}
	return da < db, true
}
