package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

func TestEnglishCatalogCoversLocalizedMessages(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	var missing []string
	seen := make(map[string]bool)

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			index := localizedMessageArgument(call)
			if index < 0 || index >= len(call.Args) {
				return true
			}
			literal, ok := call.Args[index].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(literal.Value)
			if err != nil || !hasHan(value) || seen[value] {
				return true
			}
			seen[value] = true
			if _, ok := english[value]; !ok {
				missing = append(missing, value)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan localized messages: %v", err)
	}
	if len(missing) > 0 {
		t.Fatalf("English catalog is missing %d messages:\n%s", len(missing), strings.Join(missing, "\n"))
	}
}

func localizedMessageArgument(call *ast.CallExpr) int {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if ok {
		pkg, pkgOK := selector.X.(*ast.Ident)
		if pkgOK && pkg.Name == "i18n" {
			switch selector.Sel.Name {
			case "T", "New", "Errorf":
				return 0
			case "TFor", "NewFor", "ErrorfFor":
				return 1
			}
		}
	}
	identifier, ok := call.Fun.(*ast.Ident)
	if ok && identifier.Name == "printErrorf" {
		return 1
	}
	return -1
}

func hasHan(value string) bool {
	for _, char := range value {
		if unicode.Is(unicode.Han, char) {
			return true
		}
	}
	return false
}
