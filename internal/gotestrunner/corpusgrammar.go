package gotestrunner

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
)

// CorpusArg is one value of a Go fuzz corpus file
// (testdata/fuzz/<Func>/<hash>): one of Go's native corpus types. For a fanned
// target these are the raw leaves, not the fuzzed value itself.
//
// SourceExpr is the literal exactly as the file wrote it (`"a@\x00"`, `-3`,
// `'x'`, `+Inf`), never re-rendered from the decoded value, and only ever a
// literal the grammar admits.
type CorpusArg struct {
	TypeName   string
	SourceExpr string
	// Bits: the float was written as math.Float32frombits/Float64frombits,
	// and SourceExpr is the bits literal.
	Bits bool
}

// Corpus is the value as the corpus file spells it, e.g. string("a@\x00").
func (a CorpusArg) Corpus() string {
	if a.Bits {
		if a.TypeName == "float32" {
			return "math.Float32frombits(" + a.SourceExpr + ")"
		}
		return "math.Float64frombits(" + a.SourceExpr + ")"
	}
	return a.TypeName + "(" + a.SourceExpr + ")"
}

// corpusHeader opens every corpus file Go writes.
const corpusHeader = "go test fuzz v1"

// CorpusError is a corpus file that is not in Go's corpus format, located at
// the line that is not.
type CorpusError struct {
	Path   string
	Line   int
	Reason string
}

func (e *CorpusError) Error() string {
	return fmt.Sprintf("%s:%d: %s", e.Path, e.Line, e.Reason)
}

// ParseCorpusFile reads a Go fuzz corpus file: the "go test fuzz v1" header,
// then one value per line in the grammar Go's internal/fuzz encoder writes
// and its decoder reads. Anything else, a call, an identifier, an operator, a
// composite literal or stray text, is a *CorpusError at its line.
func ParseCorpusFile(path string) ([]CorpusArg, error) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: a corpus file under the package's testdata
	if err != nil {
		return nil, err
	}
	lines := bytes.Split(data, []byte("\n"))
	if strings.TrimSuffix(string(lines[0]), "\r") != corpusHeader {
		return nil, &CorpusError{Path: path, Line: 1, Reason: "missing or invalid corpus header, want " + strconv.Quote(corpusHeader)}
	}
	var args []CorpusArg
	for i, raw := range lines[1:] {
		line := bytes.TrimSpace(raw)
		if len(line) == 0 {
			continue
		}
		arg, err := parseCorpusValue(line)
		if err != nil {
			return nil, &CorpusError{Path: path, Line: i + 2, Reason: fmt.Sprintf("not a corpus literal: %s: %v", line, err)}
		}
		args = append(args, arg)
	}
	if len(args) == 0 {
		return nil, &CorpusError{Path: path, Line: len(lines), Reason: "no corpus values"}
	}
	return args, nil
}

// parseCorpusValue reads one corpus line, admitting exactly what Go's
// decoder (internal/fuzz's parseCorpusValue) admits, values included: a
// literal out of its type's range is refused as the engine would.
func parseCorpusValue(line []byte) (CorpusArg, error) {
	fs := token.NewFileSet()
	expr, err := parser.ParseExprFrom(fs, "(corpus)", line, 0)
	if err != nil {
		return CorpusArg{}, err
	}
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return CorpusArg{}, errors.New("expected call expression")
	}
	if len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return CorpusArg{}, fmt.Errorf("expected call expression with 1 argument; got %d", len(call.Args))
	}
	arg := call.Args[0]
	src := func() string {
		return string(line[fs.Position(arg.Pos()).Offset:fs.Position(arg.End()).Offset])
	}

	if arrayType, ok := call.Fun.(*ast.ArrayType); ok {
		if arrayType.Len != nil {
			return CorpusArg{}, errors.New("expected []byte or primitive type")
		}
		elt, ok := arrayType.Elt.(*ast.Ident)
		if !ok || elt.Name != "byte" {
			return CorpusArg{}, errors.New("expected []byte")
		}
		lit, ok := arg.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return CorpusArg{}, errors.New("string literal required for type []byte")
		}
		if _, err := strconv.Unquote(lit.Value); err != nil {
			return CorpusArg{}, err
		}
		return CorpusArg{TypeName: "[]byte", SourceExpr: src()}, nil
	}

	typ := ""
	if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
		xIdent, ok := selector.X.(*ast.Ident)
		if !ok || xIdent.Name != "math" {
			return CorpusArg{}, errors.New("invalid selector type")
		}
		switch selector.Sel.Name {
		case "Float64frombits":
			typ = "float64-bits"
		case "Float32frombits":
			typ = "float32-bits"
		default:
			return CorpusArg{}, errors.New("invalid selector type")
		}
	} else {
		id, ok := call.Fun.(*ast.Ident)
		if !ok {
			return CorpusArg{}, errors.New("expected []byte or primitive type")
		}
		typ = id.Name
		if typ == "bool" {
			v, ok := arg.(*ast.Ident)
			if !ok {
				return CorpusArg{}, errors.New("malformed bool")
			}
			if v.Name != "true" && v.Name != "false" {
				return CorpusArg{}, errors.New("true or false required for type bool")
			}
			return CorpusArg{TypeName: "bool", SourceExpr: v.Name}, nil
		}
	}

	val, kind, err := corpusLiteral(arg)
	if err != nil {
		return CorpusArg{}, err
	}
	if err := checkCorpusValue(typ, val, kind); err != nil {
		return CorpusArg{}, err
	}
	switch typ {
	case "float64-bits":
		return CorpusArg{TypeName: "float64", SourceExpr: src(), Bits: true}, nil
	case "float32-bits":
		return CorpusArg{TypeName: "float32", SourceExpr: src(), Bits: true}, nil
	}
	return CorpusArg{TypeName: typ, SourceExpr: src()}, nil
}

// corpusLiteral reads the literal a numeric, string or character line holds:
// a basic literal, one negated, or the float specials Inf, +Inf, -Inf and NaN.
func corpusLiteral(arg ast.Expr) (string, token.Token, error) {
	if op, ok := arg.(*ast.UnaryExpr); ok {
		switch lit := op.X.(type) {
		case *ast.BasicLit:
			if op.Op != token.SUB {
				return "", 0, fmt.Errorf("unsupported operation on int/float: %v", op.Op)
			}
			return op.Op.String() + lit.Value, lit.Kind, nil
		case *ast.Ident:
			if lit.Name != "Inf" {
				return "", 0, errors.New("expected operation on int or float type")
			}
			if op.Op == token.SUB {
				return "-Inf", token.FLOAT, nil
			}
			if op.Op != token.ADD {
				return "", 0, fmt.Errorf("unsupported operation on Inf: %v", op.Op)
			}
			return "+Inf", token.FLOAT, nil
		default:
			return "", 0, errors.New("expected operation on int or float type")
		}
	}
	switch lit := arg.(type) {
	case *ast.BasicLit:
		return lit.Value, lit.Kind, nil
	case *ast.Ident:
		if lit.Name != "NaN" {
			return "", 0, errors.New("literal value required for primitive type")
		}
		return "NaN", token.FLOAT, nil
	}
	return "", 0, errors.New("literal value required for primitive type")
}

// checkCorpusValue checks a literal against its corpus type as Go's decoder
// does, parsing it to make sure it fits.
func checkCorpusValue(typ, val string, kind token.Token) error {
	switch typ {
	case "string":
		if kind != token.STRING {
			return errors.New("string literal value required for type string")
		}
		_, err := strconv.Unquote(val)
		return err
	case "byte", "rune":
		if kind == token.INT {
			if typ == "rune" {
				_, err := strconv.ParseInt(val, 0, 32)
				return err
			}
			_, err := strconv.ParseUint(val, 0, 8)
			return err
		}
		if kind != token.CHAR {
			return errors.New("character literal required for byte/rune types")
		}
		n := len(val)
		if n < 2 {
			return errors.New("malformed character literal, missing single quotes")
		}
		code, _, _, err := strconv.UnquoteChar(val[1:n-1], '\'')
		if err != nil {
			return err
		}
		if typ == "byte" && code >= 256 {
			return errors.New("can only encode single byte to a byte type")
		}
		return nil
	case "int", "int8", "int16", "int32", "int64":
		if kind != token.INT {
			return errors.New("integer literal required for int types")
		}
		_, err := strconv.ParseInt(val, 0, intBits(typ))
		return err
	case "uint", "uint8", "uint16", "uint32", "uint64":
		if kind != token.INT {
			return errors.New("integer literal required for uint types")
		}
		_, err := strconv.ParseUint(val, 0, intBits(typ))
		return err
	case "float32", "float64":
		if kind != token.FLOAT && kind != token.INT {
			return fmt.Errorf("float or integer literal required for %s type", typ)
		}
		bits := 64
		if typ == "float32" {
			bits = 32
		}
		_, err := strconv.ParseFloat(val, bits)
		return err
	case "float32-bits":
		if kind != token.INT {
			return errors.New("integer literal required for math.Float32frombits type")
		}
		_, err := strconv.ParseUint(val, 0, 32)
		return err
	case "float64-bits":
		if kind != token.FLOAT && kind != token.INT {
			return errors.New("integer literal required for math.Float64frombits type")
		}
		_, err := strconv.ParseUint(val, 0, 64)
		return err
	}
	return errors.New("expected []byte or primitive type")
}

// intBits is the bit size Go's decoder parses an integer type with; int and
// uint take 64 bits whatever the platform, as the decoder does.
func intBits(typ string) int {
	switch strings.TrimPrefix(typ, "u") {
	case "int8":
		return 8
	case "int16":
		return 16
	case "int32":
		return 32
	}
	return 64
}
