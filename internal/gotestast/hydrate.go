package gotestast

import (
	"go/ast"
	"go/types"
)

// ClassifyLocalFields analyzes a Hydrate method's AST to determine which
// exported fixture fields are assigned (directly or via one-level-deep receiver
// method calls). Fields assigned in Hydrate are "local" — they are excluded
// from JSON serialization and reconstructed in the test process.
func ClassifyLocalFields(f *FixtureSpec) map[string]bool {
	if f.HydrateDecl == nil || f.HydrateDecl.Body == nil {
		return nil
	}

	recvName := receiverName(f.HydrateDecl)
	if recvName == "" {
		return nil
	}

	local := make(map[string]bool)

	collectAssignments(f.HydrateDecl.Body, recvName, local)

	for _, name := range collectReceiverMethodCalls(f.HydrateDecl.Body, recvName) {
		body := findMethodBody(f, name)
		if body == nil {
			continue
		}
		collectAssignments(body, recvName, local)
	}

	if len(local) == 0 {
		return nil
	}
	return local
}

func receiverName(decl *ast.FuncDecl) string {
	if decl.Recv == nil || len(decl.Recv.List) == 0 {
		return ""
	}
	names := decl.Recv.List[0].Names
	if len(names) == 0 {
		return ""
	}
	return names[0].Name
}

// collectAssignments walks a block and records receiver field assignments
// like `f.FieldName = ...` or `f.FieldName, _ = ...`.
func collectAssignments(block *ast.BlockStmt, recvName string, local map[string]bool) {
	ast.Inspect(block, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range assign.Lhs {
			sel, ok := lhs.(*ast.SelectorExpr)
			if !ok {
				continue
			}
			ident, ok := sel.X.(*ast.Ident)
			if !ok || ident.Name != recvName {
				continue
			}
			if sel.Sel.IsExported() {
				local[sel.Sel.Name] = true
			}
		}
		return true
	})
}

// collectReceiverMethodCalls returns the names of unexported methods called on
// the receiver in the given block (e.g. `f.connect(...)` → "connect").
func collectReceiverMethodCalls(block *ast.BlockStmt, recvName string) []string {
	var names []string
	ast.Inspect(block, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Name != recvName {
			return true
		}
		names = append(names, sel.Sel.Name)
		return true
	})
	return names
}

// findMethodBody searches the fixture's package for a method with the given
// name on the fixture type and returns its body.
func findMethodBody(f *FixtureSpec, methodName string) *ast.BlockStmt {
	fixtureName := f.Identifier()
	for _, file := range f.PackageSyntax() {
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Recv == nil || fd.Name.Name != methodName {
				continue
			}
			obj := f.PackageTypesInfo().ObjectOf(fd.Name)
			fn, ok := obj.(*types.Func)
			if !ok {
				continue
			}
			sig, ok := fn.Type().(*types.Signature)
			if !ok || sig.Recv() == nil {
				continue
			}
			recv := sig.Recv().Type()
			if ptr, ok := recv.(*types.Pointer); ok {
				recv = ptr.Elem()
			}
			named, ok := recv.(*types.Named)
			if !ok || named.Obj().Name() != fixtureName {
				continue
			}
			return fd.Body
		}
	}
	return nil
}
