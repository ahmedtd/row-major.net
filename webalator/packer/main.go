// Packer assembles files into a web content pack that webalator can read.
//
// This removes the need to bundle templates and static files into the webalator
// image.  Instead, they can be packed into a single versioned snapshot,
// uploaded to GCS, and referenced at runtime.
//
// The pack format is not yet defined, but I anticipate zip + metadata files.
package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"row-major/webalator/packer/manifestpb"

	"google.golang.org/protobuf/proto"
)

var (
	output                 = flag.String("output", "", "Content pack output file.")
	staticFiles            = &StringSliceFlag{}
	staticFileTrimPrefix   = flag.String("static_file_trim_prefix", "", "Prefix to trim from static files.")
	templateFiles          = &StringSliceFlag{}
	templateFileTrimPrefix = flag.String("template_file_trim_prefix", "", "Prefix to trim from template files.")
)

func init() {
	flag.Var(staticFiles, "static_file", "A raw file to add to the content pack.")
	flag.Var(templateFiles, "template_file", "A golang html template to add to the content pack.")
}

// StringSliceFlag is a flag.Value that collects string values from multiple
// appearances on the command line.
type StringSliceFlag struct {
	Slice []string
}

// String implements flag.Value.
func (f *StringSliceFlag) String() string {
	return fmt.Sprintf("%v", f.Slice)
}

// Set implements flag.Value.
func (f *StringSliceFlag) Set(value string) error {
	f.Slice = append(f.Slice, value)
	return nil
}

func do(output string, staticFiles []string, staticFileTrimPrefix string, templateFiles []string, templateFileTrimPrefix string) error {
	fw, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("while creating output file: %w", err)
	}
	defer fw.Close()

	zw := zip.NewWriter(fw)
	defer zw.Close()

	manifest := &manifestpb.Manifest{}

	for _, path := range staticFiles {
		w, err := zw.Create(strings.TrimPrefix(path, staticFileTrimPrefix))
		if err != nil {
			return fmt.Errorf("while creating zip static file member %v: %w", path, err)
		}

		r, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("while reading static file %v: %w", path, err)
		}
		defer r.Close()

		if _, err := io.Copy(w, r); err != nil {
			return fmt.Errorf("while writing zip static file member %v: %w", path, err)
		}

		manifest.StaticFiles = append(manifest.StaticFiles, &manifestpb.StaticFile{
			Path: path,
		})
	}

	for _, path := range templateFiles {
		w, err := zw.Create(strings.TrimPrefix(path, templateFileTrimPrefix))
		if err != nil {
			return fmt.Errorf("while creating zip template member %v: %w", path, err)
		}

		r, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("while reading template %v: %w", path, err)
		}
		defer r.Close()

		if _, err := io.Copy(w, r); err != nil {
			return fmt.Errorf("while writing zip template member %v: %w", path, err)
		}

		manifest.Templates = append(manifest.Templates, &manifestpb.Template{
			Path: path,
		})
	}

	manifestBytes, err := proto.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("while marshalling manifest: %w", err)
	}

	w, err := zw.Create("manifest")
	if err != nil {
		return fmt.Errorf("while creating zip member manifest: %w", err)
	}

	if _, err := w.Write(manifestBytes); err != nil {
		return fmt.Errorf("while writing zip member manifest: %w", err)
	}

	return nil
}

func main() {
	flag.Parse()

	if err := do(*output, staticFiles.Slice, *staticFileTrimPrefix, templateFiles.Slice, *templateFileTrimPrefix); err != nil {
		log.Printf("Error: %v", err)
		os.Exit(1)
	}
}
