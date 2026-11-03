package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// openWrites are the calls that change something and are deliberately
// open to everyone who can sign in, with the reason. Everything else
// that writes has to go through adminAPI, and TestEveryWriteIsDecided
// fails when a new route is added without that decision being made.
var openWrites = map[string]string{
	"POST /api/login":    "signing in is how you stop being anonymous",
	"POST /api/logout":   "ending one's own session",
	"POST /api/password": "one's own password; the old one is asked for again",
	"POST /api/users":    "admin()'s own gate, which also refuses a fixed password file",
	"DELETE /api/users":  "same",
	"POST /api/resolve":  "reverse DNS for the names on screen; stores nothing",
}

// TestEveryWriteIsDecided reads the route table and fails on a writing
// route that is neither behind adminAPI nor listed in openWrites.
//
// It parses the source because that is the only place the whole table is
// visible: http.ServeMux will not list what was registered with it. The
// bug this guards against is not a wrong gate but a missing one — a
// route added next to the others, inheriting a wrapper that only asks
// whether the caller is signed in — so the test has to see the table as
// it is written rather than the handlers one by one.
func TestEveryWriteIsDecided(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "server.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	gated, open := map[string]bool{}, map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok || len(c.Args) == 0 {
			return true
		}
		fn, ok := c.Fun.(*ast.Ident)
		if !ok {
			return true
		}
		lit, ok := c.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		pat, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		switch fn.Name {
		case "adminAPI":
			gated[pat] = true
		case "api":
			open[pat] = true
		}
		return true
	})
	if len(gated) == 0 || len(open) == 0 {
		t.Fatalf("read no route table: %d gated, %d open", len(gated), len(open))
	}
	// mux.HandleFunc routes are registered by name, not through either
	// wrapper; they are in openWrites and checked by the same rule.
	ast.Inspect(f, func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok || len(c.Args) == 0 {
			return true
		}
		sel, ok := c.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "HandleFunc" && sel.Sel.Name != "Handle" {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); !ok || id.Name != "mux" {
			return true
		}
		if lit, ok := c.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			if pat, err := strconv.Unquote(lit.Value); err == nil {
				open[pat] = true
			}
		}
		return true
	})
	var missing []string
	for pat := range open {
		method, _, ok := strings.Cut(pat, " ")
		if !ok || method == "GET" || method == "HEAD" {
			continue
		}
		if openWrites[pat] == "" {
			missing = append(missing, pat)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("these routes change something but ask only whether the caller is signed in:\n\t%s\n"+
			"Register each with adminAPI, or add it to openWrites with the reason it is open to everyone.",
			strings.Join(missing, "\n\t"))
	}
	// A stale entry is worth hearing about too: it reads as a decision
	// that has been made when the route is gone.
	for pat := range openWrites {
		if !open[pat] && !gated[pat] {
			t.Errorf("openWrites lists %q, which the route table no longer has", pat)
		}
		if gated[pat] {
			t.Errorf("openWrites lists %q, which is behind adminAPI; drop the entry", pat)
		}
	}
}
