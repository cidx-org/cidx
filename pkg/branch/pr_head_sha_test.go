package branch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestGetPRInfoCarriesTheHeadSHA fails when GetPRInfo stops filling PRInfo.HeadSHA.
//
// The watch guard (#414) compares that field against local HEAD and refuses to
// report when they differ. An empty field is not a failure there — it means
// "could not tell", which lets the watch run and says so, because refusing every
// watch over an unreadable SHA would be worse than the hole it closes.
//
// That tolerance is what makes this test necessary. If the assignment were
// dropped, nothing would break: no test would go red, no run would error, and
// every watch would quietly print "could not verify" forever while reporting
// exactly what it reported before the guard existed. A guard that degrades into
// a no-op without saying so is the failure mode the guard was written against.
//
// No behavioural test can see this: GetPRInfo talks to a *github.Client with no
// interface in front of it, so the field's origin is only visible in the source.
func TestGetPRInfoCarriesTheHeadSHA(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "list.go", nil, 0)
	if err != nil {
		t.Fatalf("failed to parse list.go: %v", err)
	}

	literal := findPRInfoLiteral(t, file, "GetPRInfo")
	for _, element := range literal.Elts {
		if kv, ok := element.(*ast.KeyValueExpr); ok {
			if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "HeadSHA" {
				return
			}
		}
	}

	t.Errorf("GetPRInfo builds a PRInfo without HeadSHA — the watch guard (#414) reads that field, "+
		"and an empty one makes it report \"could not verify\" on every watch instead of failing.\n"+
		"Restore it with: HeadSHA: pr.GetHead().GetSHA(), at %s", fset.Position(literal.Pos()))
}

// findPRInfoLiteral returns the PRInfo composite literal built inside fn.
func findPRInfoLiteral(t *testing.T, file *ast.File, fn string) *ast.CompositeLit {
	t.Helper()

	var found *ast.CompositeLit
	for _, decl := range file.Decls {
		decl, ok := decl.(*ast.FuncDecl)
		if !ok || decl.Name.Name != fn {
			continue
		}

		ast.Inspect(decl, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			if ident, ok := lit.Type.(*ast.Ident); ok && ident.Name == "PRInfo" {
				found = lit
				return false
			}
			return true
		})
	}

	if found == nil {
		t.Fatalf("no PRInfo literal found in %s — this test would pass by watching nothing", fn)
	}
	return found
}
