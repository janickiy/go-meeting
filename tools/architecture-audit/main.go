// Command architecture-audit inventories production Go declarations under
// internal/. It measures source structure, not runtime coupling or complexity.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type function struct {
	Name       string
	File       string
	Line       int
	LOC        int
	Branches   int // if, for, range, case, comm nodes, including closures; not cyclomatic complexity
	Parameters int
}

type packageInfo struct {
	Path      string
	Files     []string
	Imports   []string
	Inbound   []string
	Public    []string
	Types     map[string][]string
	Functions []function
}

func expression(fset *token.FileSet, node ast.Node) string {
	var buf bytes.Buffer
	_ = printer.Fprint(&buf, fset, node)
	return buf.String()
}

func fieldNames(fset *token.FileSet, fields *ast.FieldList) []string {
	result := []string{}
	for _, field := range fields.List {
		kind := expression(fset, field.Type)
		if len(field.Names) == 0 {
			result = append(result, "embedded: "+kind)
		}
		for _, name := range field.Names {
			result = append(result, name.Name+": "+kind)
		}
	}
	return result
}

func inventory(root string) ([]*packageInfo, error) {
	fset := token.NewFileSet()
	packages := map[string]*packageInfo{}
	err := filepath.WalkDir(filepath.Join(root, "internal"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		key := filepath.ToSlash(filepath.Dir(rel))
		p := packages[key]
		if p == nil {
			p = &packageInfo{Path: key, Types: map[string][]string{}}
			packages[key] = p
		}
		p.Files = append(p.Files, rel)
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			name, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return err
			}
			p.Imports = append(p.Imports, name)
		}
		for _, decl := range file.Decls {
			switch v := decl.(type) {
			case *ast.FuncDecl:
				name := v.Name.Name
				if v.Recv != nil {
					name = expression(fset, v.Recv.List[0].Type) + "." + name
				}
				f := function{Name: name, File: rel, Line: fset.Position(v.Pos()).Line, LOC: fset.Position(v.End()).Line - fset.Position(v.Pos()).Line + 1}
				for _, arg := range v.Type.Params.List {
					n := len(arg.Names)
					if n == 0 {
						n = 1
					}
					f.Parameters += n
				}
				ast.Inspect(v.Body, func(n ast.Node) bool {
					switch n.(type) {
					case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.CaseClause, *ast.CommClause:
						f.Branches++
					}
					return true
				})
				p.Functions = append(p.Functions, f)
				if v.Name.IsExported() {
					p.Public = append(p.Public, name)
				}
			case *ast.GenDecl:
				for _, spec := range v.Specs {
					if value, ok := spec.(*ast.ValueSpec); ok {
						for _, name := range value.Names {
							if name.IsExported() {
								p.Public = append(p.Public, v.Tok.String()+" "+name.Name)
							}
						}
						continue
					}
					t, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					switch kind := t.Type.(type) {
					case *ast.StructType:
						p.Types[t.Name.Name+" struct"] = fieldNames(fset, kind.Fields)
					case *ast.InterfaceType:
						p.Types[t.Name.Name+" interface"] = fieldNames(fset, kind.Methods)
					default:
						p.Types[t.Name.Name] = []string{expression(fset, t.Type)}
					}
					if t.Name.IsExported() {
						p.Public = append(p.Public, "type "+t.Name.Name)
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	result := make([]*packageInfo, 0, len(packages))
	for _, p := range packages {
		p.Imports = unique(p.Imports)
		p.Public = unique(p.Public)
		for _, imp := range p.Imports {
			if i := strings.Index(imp, "/internal/"); i >= 0 {
				if dep := packages[imp[i+1:]]; dep != nil {
					dep.Inbound = append(dep.Inbound, p.Path)
				}
			}
		}
		sort.Slice(p.Functions, func(i, j int) bool {
			if p.Functions[i].LOC == p.Functions[j].LOC {
				return p.Functions[i].Name < p.Functions[j].Name
			}
			return p.Functions[i].LOC > p.Functions[j].LOC
		})
		result = append(result, p)
	}
	for _, p := range result {
		p.Inbound = unique(p.Inbound)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}

func unique(values []string) []string {
	sort.Strings(values)
	result := []string{}
	for _, v := range values {
		if len(result) == 0 || result[len(result)-1] != v {
			result = append(result, v)
		}
	}
	return result
}

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	result, err := inventory(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
