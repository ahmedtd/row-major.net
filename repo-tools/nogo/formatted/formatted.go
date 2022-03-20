// Package formatted is a nogo analyzer that checks that all Go source
// files are formatted by gofmt.
package formatted

import (
	"bytes"
	"go/format"
	"io/ioutil"

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

		in, err := ioutil.ReadFile(filename)
		if err != nil {
			return nil, err
		}

		out, err := format.Source(in)
		if err != nil {
			return nil, err
		}

		if bytes.Equal(in, out) {
			continue
		}

		pass.Reportf(file.Pos(), "File is incorrectly formatted; please run `gofmt`")
	}
	return nil, nil
}
