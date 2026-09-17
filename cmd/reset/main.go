package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type foundStruct struct {
	name    string
	pkgName string
	fields  *ast.FieldList
}

func main() {
	dir, err := findProjectRoot()
	if err != nil {
		log.Fatal(err.Error())
	}

	fileSet := token.NewFileSet()
	groups := make(map[string][]foundStruct)

	err = filepath.WalkDir(
		dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}

			if d.IsDir() && d.Name() == ".git" {
				return filepath.SkipDir
			}
			if strings.HasPrefix(d.Name(), ".") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			if d.IsDir() {
				return nil
			}

			if strings.HasSuffix(d.Name(), "_test.go") {
				return nil
			}

			if !strings.HasSuffix(d.Name(), ".go") {
				return nil
			}
			if d.Name() == "reset.gen.go" {
				return nil
			}

			file, err := parser.ParseFile(fileSet, path, nil, parser.ParseComments)
			if err != nil {
				log.Printf("skipping %s: %v", path, err)
				return nil
			}

			for _, found := range scanFileForResetStructs(file) {
				groups[filepath.Dir(path)] = append(groups[filepath.Dir(path)], found)
			}

			return nil
		},
	)
	if err != nil {
		log.Fatal(err.Error())
	}

	for dir, structs := range groups {
		content, genErr := generateFile(structs[0].pkgName, structs)
		if genErr != nil {
			log.Printf("failed to generate reset.gen.go for %s: %v", dir, genErr)
			continue
		}

		if writeErr := writeGeneratedFile(dir, content); writeErr != nil {
			log.Printf("failed to write reset.gen.go for %s: %v", dir, writeErr)
		}
	}
}

func scanFileForResetStructs(file *ast.File) []foundStruct {
	var found []foundStruct

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		if genDecl.Doc == nil || !strings.Contains(genDecl.Doc.Text(), "generate:reset") {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}

			found = append(
				found, foundStruct{
					name:    typeSpec.Name.Name,
					pkgName: file.Name.Name,
					fields:  structType.Fields,
				},
			)
		}
	}

	return found
}

func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir = filepath.Clean(dir)

	for {
		target := filepath.Join(dir, "go.mod")
		if fi, err := os.Stat(target); err == nil && !fi.IsDir() {
			return dir, nil
		}

		parentDir := filepath.Dir(dir)
		if parentDir == dir {
			break
		}
		dir = parentDir
	}

	return "", errors.New("project root not found")
}

// fieldNames returns the names a struct field is accessed by. For a normal
// field ("status int") that's just its declared name(s). For an embedded
// field with no name of its own ("http.ResponseWriter", "sync.Mutex") Go
// promotes the last component of the type as the field's name, so we derive
// it from the type expression instead.
func fieldNames(field *ast.Field) []string {
	if len(field.Names) > 0 {
		names := make([]string, 0, len(field.Names))
		for _, n := range field.Names {
			names = append(names, n.Name)
		}
		return names
	}

	switch t := field.Type.(type) {
	case *ast.Ident:
		return []string{t.Name}
	case *ast.SelectorExpr:
		return []string{t.Sel.Name}
	case *ast.StarExpr:
		switch inner := t.X.(type) {
		case *ast.Ident:
			return []string{inner.Name}
		case *ast.SelectorExpr:
			return []string{inner.Sel.Name}
		}
	}

	return nil
}

var primitiveZero = map[string]string{
	"int":     "0",
	"int8":    "0",
	"int16":   "0",
	"int32":   "0",
	"int64":   "0",
	"uint":    "0",
	"uint8":   "0",
	"uint16":  "0",
	"uint32":  "0",
	"uint64":  "0",
	"float32": "0",
	"float64": "0",
	"string":  `""`,
	"bool":    "false",
	"byte":    "0",
	"rune":    "0",
}

func generateResetMethod(s foundStruct) string {
	var b strings.Builder

	_, _ = fmt.Fprintf(&b, "func (r *%s) Reset() {", s.name)
	b.WriteString("if r == nil { return };")

	for _, field := range s.fields.List {
		names := fieldNames(field)

		if ident, ok := field.Type.(*ast.Ident); ok {
			if zero, isPrimitive := primitiveZero[ident.Name]; isPrimitive {
				for _, name := range names {
					_, _ = fmt.Fprintf(&b, "r.%s = %s;", name, zero)
				}
				continue
			}
		}

		if starExpr, ok := field.Type.(*ast.StarExpr); ok {
			if innerIdent, ok := starExpr.X.(*ast.Ident); ok {
				if zero, isPrimitive := primitiveZero[innerIdent.Name]; isPrimitive {
					for _, name := range names {
						_, _ = fmt.Fprintf(&b, "if r.%s != nil { *r.%s = %s };", name, name, zero)
					}
					continue
				}
			}
			for _, name := range names {
				_, _ = fmt.Fprintf(
					&b,
					"if r.%s != nil { if resetter, ok := any(r.%s).(interface{ Reset() }); ok { resetter.Reset() } };",
					name, name,
				)
			}
			continue
		}

		if _, ok := field.Type.(*ast.ArrayType); ok {
			for _, name := range names {
				_, _ = fmt.Fprintf(&b, "r.%s = r.%s[:0];", name, name)
			}
			continue
		}

		if _, ok := field.Type.(*ast.MapType); ok {
			for _, name := range names {
				_, _ = fmt.Fprintf(&b, "clear(r.%s);", name)
			}
			continue
		}

		for _, name := range names {
			_, _ = fmt.Fprintf(
				&b,
				"if resetter, ok := any(&r.%s).(interface{ Reset() }); ok { resetter.Reset() };",
				name,
			)
		}
	}

	b.WriteString("}")
	return b.String()
}

func generateFile(pkgName string, structs []foundStruct) ([]byte, error) {
	var b strings.Builder

	b.WriteString("// Code generated by cmd/reset. DO NOT EDIT.\n\n")
	_, _ = fmt.Fprintf(&b, "package %s\n\n", pkgName)

	for _, s := range structs {
		b.WriteString(generateResetMethod(s))
		b.WriteString("\n\n")
	}

	return format.Source([]byte(b.String()))
}

func writeGeneratedFile(dir string, content []byte) (err error) {
	file, err := os.Create(filepath.Join(dir, "reset.gen.go"))
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	_, err = file.Write(content)
	return err
}
