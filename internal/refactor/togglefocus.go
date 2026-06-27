package refactor

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
)

type replacement struct {
	pos     int
	oldName string
	newName string
}

func ToggleFocus(filePath string, identifier string) error {
	src, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", filePath, err)
	}

	parts := strings.SplitN(identifier, ".", 2)
	suiteName := parts[0]
	methodName := ""
	if len(parts) == 2 {
		methodName = parts[1]
	}

	if methodName != "" {
		return toggleMethod(filePath, src, fset, file, suiteName, methodName)
	}
	return toggleSuite(filePath, src, fset, file, suiteName)
}

func toggleSuite(filePath string, src []byte, fset *token.FileSet, file *ast.File, suiteName string) error {
	newName := togglePrefix(suiteName)

	var found bool
	var replacements []replacement

	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || ts.Name.Name != suiteName {
				continue
			}
			found = true
			replacements = append(replacements, replacement{
				pos:     fset.Position(ts.Name.Pos()).Offset,
				oldName: suiteName,
				newName: newName,
			})
		}
	}

	if !found {
		return fmt.Errorf("type %s not found in %s", suiteName, filePath)
	}

	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 {
			continue
		}
		recvType := receiverTypeName(fd.Recv.List[0].Type)
		if recvType != suiteName {
			continue
		}
		pos := receiverNameOffset(fset, fd.Recv.List[0].Type)
		if pos >= 0 {
			replacements = append(replacements, replacement{
				pos:     pos,
				oldName: suiteName,
				newName: newName,
			})
		}
	}

	return applyReplacements(filePath, src, replacements)
}

func toggleMethod(filePath string, src []byte, fset *token.FileSet, file *ast.File, suiteName, methodName string) error {
	newName := togglePrefix(methodName)

	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Recv == nil || len(fd.Recv.List) == 0 {
			continue
		}
		recvType := receiverTypeName(fd.Recv.List[0].Type)
		if recvType != suiteName {
			continue
		}
		if fd.Name.Name != methodName {
			continue
		}
		return applyReplacements(filePath, src, []replacement{{
			pos:     fset.Position(fd.Name.Pos()).Offset,
			oldName: methodName,
			newName: newName,
		}})
	}

	return fmt.Errorf("method %s.%s not found in %s", suiteName, methodName, filePath)
}

func togglePrefix(name string) string {
	if strings.HasPrefix(name, "F_") {
		return name[2:]
	}
	return "F_" + name
}

func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	case *ast.Ident:
		return t.Name
	}
	return ""
}

func receiverNameOffset(fset *token.FileSet, expr ast.Expr) int {
	switch t := expr.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return fset.Position(ident.Pos()).Offset
		}
	case *ast.Ident:
		return fset.Position(t.Pos()).Offset
	}
	return -1
}

func applyReplacements(filePath string, src []byte, repls []replacement) error {
	sort.Slice(repls, func(i, j int) bool {
		return repls[i].pos > repls[j].pos
	})

	result := make([]byte, len(src))
	copy(result, src)

	for _, r := range repls {
		end := r.pos + len(r.oldName)
		if end > len(result) || string(result[r.pos:end]) != r.oldName {
			return fmt.Errorf("unexpected content at offset %d: expected %q", r.pos, r.oldName)
		}
		result = append(result[:r.pos], append([]byte(r.newName), result[end:]...)...)
	}

	return os.WriteFile(filePath, result, 0644) //nolint:gosec // G306: not sensitive data
}
