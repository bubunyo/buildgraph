package analyzer

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/bubunyo/buildgraph/pkg/config"
	"github.com/bubunyo/buildgraph/pkg/types"
)

type Analyzer struct {
	cfg        *config.Config
	rootModule string
	packages   map[string]*types.Package
	funcASTs   map[string]*ast.FuncDecl
	fset       *token.FileSet
}

func New(cfg *config.Config, rootModule string) *Analyzer {
	return &Analyzer{
		cfg:        cfg,
		rootModule: rootModule,
		packages:   make(map[string]*types.Package),
		funcASTs:   make(map[string]*ast.FuncDecl),
		fset:       token.NewFileSet(),
	}
}

func (a *Analyzer) DiscoverPackages(rootPath string) error {
	dirs := []string{
		filepath.Join(rootPath, a.cfg.Directories.Services),
		filepath.Join(rootPath, a.cfg.Directories.Core),
	}

	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				goModPath := filepath.Join(path, "go.mod")
				if _, err := os.Stat(goModPath); err == nil {
					pkg, err := a.parseGoMod(goModPath, rootPath)
					if err != nil {
						return err
					}
					a.packages[pkg.Path] = pkg
				}
			}

			return nil
		})

		if err != nil {
			return err
		}
	}

	return nil
}

func (a *Analyzer) parseGoMod(goModPath, rootPath string) (*types.Package, error) {
	content, err := os.ReadFile(goModPath)
	if err != nil {
		return nil, err
	}

	modulePath := extractModulePath(string(content))
	relPath, _ := filepath.Rel(rootPath, filepath.Dir(goModPath))
	relPath = filepath.ToSlash(relPath)

	fullPath := a.rootModule + "/" + relPath

	pkg := &types.Package{
		Path:   fullPath,
		Name:   filepath.Base(relPath),
		Module: modulePath,
	}

	return pkg, nil
}

func extractModulePath(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

func (a *Analyzer) ParseSourceFiles(rootPath string) (map[string]*types.Function, error) {
	functions := make(map[string]*types.Function)

	dirs := []string{
		filepath.Join(rootPath, a.cfg.Directories.Services),
		filepath.Join(rootPath, a.cfg.Directories.Core),
	}

	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				if a.cfg.Exclude.SkipVendor && info.Name() == "vendor" {
					return filepath.SkipDir
				}
				return nil
			}

			if !strings.HasSuffix(info.Name(), ".go") {
				return nil
			}

			if a.cfg.Exclude.SkipTests && strings.HasSuffix(info.Name(), "_test.go") {
				return nil
			}

			for _, pattern := range a.cfg.Exclude.Patterns {
				matched, _ := filepath.Match(pattern, info.Name())
				if matched {
					return nil
				}
			}

			funcs, err := a.parseFile(path, rootPath)
			if err != nil {
				return err
			}

			for _, fn := range funcs {
				functions[fn.FullName] = fn
			}

			return nil
		})

		if err != nil {
			return nil, err
		}
	}

	return functions, nil
}

func (a *Analyzer) parseFile(filePath, rootPath string) ([]*types.Function, error) {
	file, err := parser.ParseFile(a.fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(rootPath, filePath)
	relPath = filepath.ToSlash(relPath)

	pkgPath := a.determinePackage(filePath, rootPath)
	if pkgPath == "" {
		return nil, nil
	}

	var functions []*types.Function

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			if node.Name == nil {
				return true
			}

			isMain := node.Name.Name == "main" && file.Name.Name == "main"
			isExported := token.IsExported(node.Name.Name)

			fn := &types.Function{
				Name:       node.Name.Name,
				FullName:   fmt.Sprintf("%s.%s", pkgPath, node.Name.Name),
				Package:    pkgPath,
				File:       relPath,
				IsExported: isExported,
				IsMain:     isMain,
			}

			if node.Type != nil && node.Type.Params != nil {
				fn.StartLine = a.fset.Position(node.Type.Params.Pos()).Line
				fn.EndLine = a.fset.Position(node.Type.Params.End()).Line
			}

			if node.Body != nil {
				if node.Pos() != token.NoPos {
					fn.StartLine = a.fset.Position(node.Pos()).Line
				}
				if node.End() != token.NoPos {
					fn.EndLine = a.fset.Position(node.End()).Line
				}
			}

			functions = append(functions, fn)
			a.funcASTs[fn.FullName] = node
		}
		return true
	})

	return functions, nil
}

func (a *Analyzer) determinePackage(filePath, rootPath string) string {
	fileDir := filepath.Dir(filePath)
	moduleDir := fileDir

	// Find the module root (where go.mod is)
	for {
		goModPath := filepath.Join(moduleDir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			break
		}
		parent := filepath.Dir(moduleDir)
		if parent == moduleDir {
			break
		}
		moduleDir = parent
	}

	// Calculate relative path from module root to the file's directory
	relPath, err := filepath.Rel(moduleDir, fileDir)
	if err != nil {
		relPath, _ = filepath.Rel(rootPath, fileDir)
	}
	relPath = filepath.ToSlash(relPath)

	if relPath == "." || relPath == "" {
		return a.rootModule
	}
	return a.rootModule + "/" + relPath
}

func (a *Analyzer) ExtractDependencies(functions map[string]*types.Function) error {
	for _, fn := range functions {
		astNode, ok := a.funcASTs[fn.FullName]
		if !ok || astNode.Body == nil {
			continue
		}

		var deps []types.Dependency

		ast.Inspect(astNode.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			dep := a.resolveCall(call, fn.Package)
			if dep.FullName != "" {
				deps = append(deps, dep)
			}

			return true
		})

		fn.Deps = deps
	}

	return nil
}

func (a *Analyzer) resolveCall(call *ast.CallExpr, currentPkg string) types.Dependency {
	var dep types.Dependency

	switch fun := call.Fun.(type) {
	case *ast.Ident:
		localName := fun.Name

		dep = types.Dependency{
			Name:     localName,
			FullName: currentPkg + "." + localName,
			Package: types.Package{
				Path: currentPkg,
				Name: filepath.Base(currentPkg),
			},
			Type: "internal",
		}

	case *ast.SelectorExpr:
		if ident, ok := fun.X.(*ast.Ident); ok {
			pkgName := ident.Name

			if pkg, ok := a.packages[pkgName]; ok {
				dep = types.Dependency{
					Name:     fun.Sel.Name,
					FullName: pkg.Path + "." + fun.Sel.Name,
					Package:  *pkg,
					Type:     "external",
				}

				if strings.HasPrefix(pkg.Path, a.rootModule) {
					dep.Type = "internal"
					dep.FullName = pkg.Path + "." + fun.Sel.Name
				}
			}
		}
	}

	return dep
}

func (a *Analyzer) BuildIndices(functions map[string]*types.Function) *types.CallGraph {
	nodes := make(map[string]types.Function)
	reverseIndex := make(map[string][]string)
	functionOwner := make(map[string]string)

	for _, fn := range functions {
		nodes[fn.FullName] = *fn

		owner := a.extractOwner(fn.Package)
		functionOwner[fn.FullName] = owner

		for _, dep := range fn.Deps {
			if dep.Type == "internal" {
				reverseIndex[dep.FullName] = append(reverseIndex[dep.FullName], fn.FullName)
			}
		}
	}

	return &types.CallGraph{
		Nodes:         nodes,
		ReverseIndex:  reverseIndex,
		FunctionOwner: functionOwner,
	}
}

func (a *Analyzer) extractOwner(pkgPath string) string {
	relPath := strings.TrimPrefix(pkgPath, a.rootModule+"/")
	parts := strings.Split(relPath, "/")

	if len(parts) > 0 {
		return parts[0]
	}

	return pkgPath
}

func (a *Analyzer) ComputeHashes(functions map[string]*types.Function) error {
	for _, fn := range functions {
		hash, err := a.computeASTHash(fn.FullName)
		if err != nil {
			return err
		}
		fn.ASTHash = hash
	}

	return nil
}

func (a *Analyzer) computeASTHash(fullName string) (string, error) {
	astNode, ok := a.funcASTs[fullName]
	if !ok {
		return "", nil
	}

	if astNode.Body == nil {
		return "", nil
	}

	var buf bytes.Buffer
	err := format.Node(&buf, a.fset, astNode.Body)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(buf.Bytes())
	return fmt.Sprintf("sha256:%x", hash), nil
}

func (a *Analyzer) GetPackages() map[string]*types.Package {
	return a.packages
}
