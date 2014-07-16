package impl

import (
	"go/build"
)

var (
	pkgCache = map[string]*build.Package{}
	ctx      = build.Default
)

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
