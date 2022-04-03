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
	"mime"
	"os"
	"path"
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
	templateBaseFile       = flag.String("template_base_file", "", "Base file for all templates.")
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

type Packer struct {
	Output string

	StaticFiles          []string
	StaticFileTrimPrefix string

	TemplateFiles          []string
	TemplateFileTrimPrefix string
	TemplateBaseFile       string
}

func (p *Packer) Do() error {
	fw, err := os.Create(p.Output)
	if err != nil {
		return fmt.Errorf("while creating output file: %w", err)
	}
	defer fw.Close()

	zw := zip.NewWriter(fw)
	defer zw.Close()

	manifest := &manifestpb.Manifest{}

	if err := p.addStatics(zw, manifest); err != nil {
		return fmt.Errorf("while adding statics: %w", err)
	}

	// Add single base template to content pack.
	//
	// TODO: Support building content packs with multiple base templates.  The
	// manifest format already supports it, we just need packer and the Bazel
	// rules to let us specify a per-template base.
	if err := p.addBaseTemplate(zw, manifest); err != nil {
		return fmt.Errorf("while adding base template: %w", err)
	}

	if err := p.addGoTemplates(zw, manifest); err != nil {
		return fmt.Errorf("while adding go templates: %w", err)
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

func (p *Packer) addStatics(zw *zip.Writer, manifest *manifestpb.Manifest) error {
	for _, staticPath := range p.StaticFiles {
		trimmedPath := strings.TrimPrefix(staticPath, p.StaticFileTrimPrefix)

		w, err := zw.Create(trimmedPath)
		if err != nil {
			return fmt.Errorf("while creating zip static file member %v: %w", trimmedPath, err)
		}

		r, err := os.Open(staticPath)
		if err != nil {
			return fmt.Errorf("while reading static file %v: %w", staticPath, err)
		}
		defer r.Close()

		if _, err := io.Copy(w, r); err != nil {
			return fmt.Errorf("while writing zip static file member %v: %w", staticPath, err)
		}

		manifest.Servables = append(manifest.Servables, &manifestpb.Servable{
			Entry: &manifestpb.Servable_Static{
				Static: &manifestpb.Static{
					ServingPath:     "/" + trimmedPath,
					ContentPackPath: trimmedPath,
					MimeType:        mime.TypeByExtension(path.Ext(trimmedPath)),
				},
			},
		})
	}

	return nil
}

func (p *Packer) addBaseTemplate(zw *zip.Writer, manifest *manifestpb.Manifest) error {
	trimmedPath := strings.TrimPrefix(p.TemplateBaseFile, p.TemplateFileTrimPrefix)

	w, err := zw.Create(trimmedPath)
	if err != nil {
		return fmt.Errorf("while creating zip member %v: %w", trimmedPath, err)
	}

	r, err := os.Open(p.TemplateBaseFile)
	if err != nil {
		return fmt.Errorf("while reading %v: %w", p.TemplateBaseFile, err)
	}
	defer r.Close()

	if _, err := io.Copy(w, r); err != nil {
		return fmt.Errorf("while writing zip member %v: %w", trimmedPath, err)
	}

	// Don't add the base template to the manifest.

	return nil
}

func (p *Packer) addGoTemplates(zw *zip.Writer, manifest *manifestpb.Manifest) error {
	for _, templatePath := range p.TemplateFiles {
		trimmedPath := strings.TrimPrefix(templatePath, p.TemplateFileTrimPrefix)

		w, err := zw.Create(trimmedPath)
		if err != nil {
			return fmt.Errorf("while creating zip template member %v: %w", trimmedPath, err)
		}

		r, err := os.Open(templatePath)
		if err != nil {
			return fmt.Errorf("while reading template %v: %w", templatePath, err)
		}
		defer r.Close()

		if _, err := io.Copy(w, r); err != nil {
			return fmt.Errorf("while writing zip template member %v: %w", templatePath, err)
		}

		servingPath := ""
		if path.Base(trimmedPath) == "index.html.tmpl" {
			if path.Dir(trimmedPath) == "." {
				servingPath = "/"
			} else {
				servingPath = "/" + path.Dir(trimmedPath) + "/"
			}
		} else if strings.HasSuffix(trimmedPath, ".tmpl") {
			servingPath = strings.TrimSuffix(trimmedPath, ".tmpl")
		} else {
			return fmt.Errorf("template %v doesn't have extension .tmpl", templatePath)
		}

		manifest.Servables = append(manifest.Servables, &manifestpb.Servable{
			Entry: &manifestpb.Servable_GoTemplate{
				GoTemplate: &manifestpb.GoTemplate{
					ServingPath:                   servingPath,
					BaseContentPackPath:           strings.TrimPrefix(p.TemplateBaseFile, p.TemplateFileTrimPrefix),
					SpecializationContentPackPath: trimmedPath,
				},
			},
		})

		// If servingPath ends in `/`, register a redirect for the non-`/` version.
		if strings.HasSuffix(servingPath, "/") {
			manifest.Servables = append(manifest.Servables, &manifestpb.Servable{
				Entry: &manifestpb.Servable_Redirect{
					Redirect: &manifestpb.Redirect{
						ServingPath: strings.TrimSuffix(servingPath, "/"),
						Location:    servingPath,
					},
				},
			})
		}
	}

	return nil
}

func main() {
	flag.Parse()

	p := &Packer{
		Output:                 *output,
		StaticFiles:            staticFiles.Slice,
		StaticFileTrimPrefix:   *staticFileTrimPrefix,
		TemplateFiles:          templateFiles.Slice,
		TemplateFileTrimPrefix: *templateFileTrimPrefix,
		TemplateBaseFile:       *templateBaseFile,
	}

	if err := p.Do(); err != nil {
		log.Printf("Error: %v", err)
		os.Exit(1)
	}
}
