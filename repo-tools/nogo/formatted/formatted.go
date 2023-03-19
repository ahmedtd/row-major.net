// Package formatted is a nogo analyzer that checks that all Go source
// files are formatted by gofmt.
package formatted

import (
	"bytes"
	"fmt"
	"go/format"
	"io/ioutil"
	"path"

	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name: "formatted",
	Doc:  "reports unformatted files",
	Run:  run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename

		if path.Ext(filename) != "go" {
			return nil, nil
		}

		in, err := ioutil.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("while reading file: %w", err)
		}

		out, err := format.Source(in)
		if err != nil {
			return nil, fmt.Errorf("while formatting source: %w", err)
		}

		if bytes.Equal(in, out) {
			continue
		}

		pass.Reportf(file.Pos(), "File is incorrectly formatted; please run `gofmt`")
	}
	return nil, nil
}
