package impl

import (
	"go/build"
	"strings"
)

var (
	ctx                 = build.Default
	pseudoPackageC      = &build.Package{ImportPath: "C", Goroot: true}
	pseudoPackageUnsafe = &build.Package{ImportPath: "unsafe", Goroot: true}
	pkgCache            = map[string]*build.Package{"C": pseudoPackageC, "unsafe": pseudoPackageUnsafe}
)

func IsPseudoPackage(p *build.Package) bool {
	return p == pseudoPackageUnsafe || p == pseudoPackageC
}

func ImportPackage(path string) (*build.Package, error) {
	if p, ok := pkgCache[path]; ok {
		return p, nil
	}

	p, err := ctx.Import(path, "", build.AllowBinary)
	if err != nil {
		return p, err
	}

	pkgCache[path] = p
	return p, nil
}

func IsInternalPackage(p *build.Package) bool {
	for _, part := range strings.Split(p.ImportPath, "/") {
		if part == "internal" {
			return true
		}
	}
	return false
}