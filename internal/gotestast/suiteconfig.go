package gotestast

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
)

// SuiteConfigBody is the shape of a SuiteConfig method body the generator
// accepts: `return <literal|preset>`, or `cfg := <literal|preset>` followed by
// `cfg.<Field> = <value>` assignments and `return cfg`.
type SuiteConfigBody struct {
	// Base is the literal or preset call the config starts from.
	Base ast.Expr
	// Literal is Base as a composite literal; nil for a preset call.
	Literal *ast.CompositeLit
	// Holder is the statement holding Base: the return or the `cfg :=` definition.
	Holder ast.Stmt
	// CfgName is the variable the compose form defines; empty when Base is returned directly.
	CfgName string
	// Fields are the literal's keyed fields, in order.
	Fields []*ast.KeyValueExpr
	// Assigns are the compose form's `cfg.<Field> = <value>` statements, in order.
	Assigns []*ast.AssignStmt
}

// Returned reports whether Base is returned directly.
func (b *SuiteConfigBody) Returned() bool { return b.CfgName == "" }

// AssignedFields lists the field names the compose form assigns, in order.
func (b *SuiteConfigBody) AssignedFields() []string {
	names := make([]string, 0, len(b.Assigns))
	for _, assign := range b.Assigns {
		names = append(names, assign.Lhs[0].(*ast.SelectorExpr).Sel.Name)
	}
	return names
}

// ParseSuiteConfigBody reads the body of a SuiteConfig method. It returns the
// generator's error for any body the generator rejects, including a
// non-literal Parallel or Exclusive value.
func ParseSuiteConfigBody(info *types.Info, fd *ast.FuncDecl) (*SuiteConfigBody, error) {
	if fd.Body == nil || len(fd.Body.List) == 0 {
		return nil, errors.New(suiteConfigBodyErr)
	}
	stmts := fd.Body.List

	// Single-statement form: return <literal | preset call>.
	if len(stmts) == 1 {
		retStmt, ok := stmts[0].(*ast.ReturnStmt)
		if !ok || len(retStmt.Results) != 1 {
			return nil, errors.New(suiteConfigBodyErr)
		}
		body := &SuiteConfigBody{Base: retStmt.Results[0], Holder: retStmt}
		if err := body.readBase(info); err != nil {
			return nil, err
		}
		return body, nil
	}

	// Compose form: cfg := <literal|preset>; cfg.Field = value ...; return cfg.
	first, ok := stmts[0].(*ast.AssignStmt)
	if !ok || first.Tok != token.DEFINE || len(first.Lhs) != 1 || len(first.Rhs) != 1 {
		return nil, errors.New(suiteConfigBodyErr)
	}
	cfgIdent, ok := first.Lhs[0].(*ast.Ident)
	if !ok {
		return nil, errors.New(suiteConfigBodyErr)
	}
	body := &SuiteConfigBody{Base: first.Rhs[0], Holder: first, CfgName: cfgIdent.Name}
	if err := body.readBase(info); err != nil {
		return nil, err
	}

	last, ok := stmts[len(stmts)-1].(*ast.ReturnStmt)
	if !ok || len(last.Results) != 1 {
		return nil, errors.New(suiteConfigBodyErr)
	}
	if retIdent, ok := last.Results[0].(*ast.Ident); !ok || retIdent.Name != cfgIdent.Name {
		return nil, errors.New(suiteConfigBodyErr)
	}

	for _, stmt := range stmts[1 : len(stmts)-1] {
		assign, ok := stmt.(*ast.AssignStmt)
		if !ok || assign.Tok != token.ASSIGN || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return nil, errors.New(suiteConfigBodyErr)
		}
		sel, ok := assign.Lhs[0].(*ast.SelectorExpr)
		if !ok {
			return nil, errors.New(suiteConfigBodyErr)
		}
		if base, ok := sel.X.(*ast.Ident); !ok || base.Name != cfgIdent.Name {
			return nil, errors.New(suiteConfigBodyErr)
		}
		if err := checkStaticValue(sel.Sel.Name, assign.Rhs[0]); err != nil {
			return nil, err
		}
		body.Assigns = append(body.Assigns, assign)
	}
	return body, nil
}

// readBase validates Base as a keyed literal or a gotest preset call.
func (b *SuiteConfigBody) readBase(info *types.Info) error {
	switch base := b.Base.(type) {
	case *ast.CompositeLit:
		for _, elt := range base.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				// A positional literal sets fields the scan below cannot see, so a
				// Parallel or Exclusive value would silently read as false.
				return fmt.Errorf("SuiteConfig literal must use keyed fields (Field: value) — Parallel and Exclusive are resolved statically from their keys")
			}
			if key, ok := kv.Key.(*ast.Ident); ok {
				if err := checkStaticValue(key.Name, kv.Value); err != nil {
					return err
				}
			}
			b.Fields = append(b.Fields, kv)
		}
		b.Literal = base
		return nil
	case *ast.CallExpr:
		if !isKnownSuitePreset(base, info) {
			return fmt.Errorf("SuiteConfig: only the gotest presets (DefaultSuiteConfig, IntegrationSuiteConfig) may be called — Parallel and Exclusive are resolved statically and a custom helper would silently drop them")
		}
		return nil
	default:
		return errors.New(suiteConfigBodyErr)
	}
}

// checkStaticValue requires a boolean literal for Parallel and Exclusive.
func checkStaticValue(field string, value ast.Expr) error {
	if field != "Parallel" && field != "Exclusive" {
		return nil
	}
	if _, ok := boolLiteral(value); !ok {
		return fmt.Errorf("SuiteConfig: %s must be assigned a boolean literal — the generator resolves it statically", field)
	}
	return nil
}

// boolLiteral reads a `true` or `false` identifier.
func boolLiteral(value ast.Expr) (val, ok bool) {
	ident, isIdent := value.(*ast.Ident)
	if !isIdent || (ident.Name != "true" && ident.Name != "false") {
		return false, false
	}
	return ident.Name == "true", true
}
