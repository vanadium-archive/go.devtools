package impl

import (
	"encoding/json"
	"errors"
	"go/build"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

type dependencyRuleTemplate struct {
	Allow *string `json:allow`
	Deny  *string `json:deny`
	// The above fields need to be pointers to be able to distinguish
	// { "allow": "test", "deny": "" } from { "allow": "test" }
	// by looking at the return value of json.Unmarshal.
}

type dependencyPolicyTemplate struct {
	Incoming []dependencyRuleTemplate `json:incoming`
	Outgoing []dependencyRuleTemplate `json:outgoing`
}

type configFileTemplate struct {
	Dependencies dependencyPolicyTemplate `json:dependencies`
}

var configCache = map[string]*PackageConfig{}

// loadPackageConfigFile loads a configuration file (GO.PACKAGE) file
// located at the specified filesystem path.  If the call is successful,
// the output will be cached and the same instance will be returned in
// the subsequent calls.
func loadPackageConfigFile(path string) (*PackageConfig, error) {
	if p, ok := configCache[path]; ok {
		return p, nil
	}

	x, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	p, err := deserializePackageConfigData(x)
	if err != nil {
		return nil, err
	}

	p.Path = path
	configCache[path] = p
	return p, nil
}

func deserializeDependencyRule(r dependencyRuleTemplate) (DependencyRule, error) {
	switch {
	case r.Allow != nil && r.Deny != nil:
		return DependencyRule{}, errors.New("invalid dependency rule: both allow and deny are specified")
	case r.Allow == nil && r.Deny == nil:
		return DependencyRule{}, errors.New("invalid dependency rule: neither allow nor deny are specified")
	case r.Allow != nil:
		return DependencyRule{IsDenyRule: false, PackageExpression: *r.Allow}, nil
	default:
		return DependencyRule{IsDenyRule: true, PackageExpression: *r.Deny}, nil
	}
}

func deserializeDependencyRules(d []dependencyRuleTemplate) ([]DependencyRule, error) {
	a := []DependencyRule{}
	for _, r := range d {
		if x, err := deserializeDependencyRule(r); err != nil {
			return nil, err
		} else {
			a = append(a, x)
		}
	}
	return a, nil
}

func deserializePackageConfigData(data []byte) (*PackageConfig, error) {
	var pkg configFileTemplate
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}
	inc, err := deserializeDependencyRules(pkg.Dependencies.Incoming)
	if err != nil {
		return nil, err
	}
	out, err := deserializeDependencyRules(pkg.Dependencies.Outgoing)
	if err != nil {
		return nil, err
	}
	return &PackageConfig{Dependencies: DependencyPolicy{inc, out}}, nil
}

type PackageConfigIterator interface {
	Advance() bool
	Value() *PackageConfig
	Err() error
}

type configFileIterator struct {
	val   *PackageConfig
	err   error
	depth int
	dir   string
}

type configFileReadError struct {
	innerError error
	path       string
}

func (e *configFileReadError) Error() string {
	return "invalid config file: " + e.path + ": " + e.innerError.Error()
}

const configFileName = "GO.PACKAGE"

func (c *configFileIterator) Advance() bool {
	if c.depth < 0 {
		return false
	}
	configFilePath := filepath.Join(c.dir, configFileName)
	pkgConfig, err := loadPackageConfigFile(configFilePath)
	if err != nil {
		if !os.IsNotExist(err) {
			c.depth = -1
			c.err = &configFileReadError{err, configFilePath}
			return false
		}

		pkgConfig = &PackageConfig{Path: configFilePath}
	}

	c.depth--
	c.dir = filepath.Dir(c.dir)
	c.val = pkgConfig
	return true
}

func (c *configFileIterator) Value() *PackageConfig {
	return c.val
}

func (c *configFileIterator) Err() error {
	return c.err
}

func NewPackageConfigFileIterator(p *build.Package) PackageConfigIterator {
	if IsPseudoPackage(p) {
		return &configFileIterator{depth: -1}
	}
	return &configFileIterator{
		dir:   p.Dir,
		depth: strings.Count(p.ImportPath, "/"),
	}
}
