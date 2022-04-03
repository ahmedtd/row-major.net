package contentpack

import (
	"archive/zip"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"row-major/webalator/packer/manifestpb"

	"google.golang.org/protobuf/proto"
)

type servable interface {
	servingPath() string
	serveHTTP(zr *zip.Reader, w http.ResponseWriter, r *http.Request) error
}

type staticServable struct {
	sp              string
	contentPackPath string
	mimeType        string
}

func (s *staticServable) servingPath() string {
	return s.sp
}

func (s *staticServable) serveHTTP(zr *zip.Reader, w http.ResponseWriter, r *http.Request) error {
	w.Header().Add("Content-Type", s.mimeType)

	zf, err := zr.Open(s.contentPackPath)
	if err != nil {
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return fmt.Errorf("while opening content pack member %v: %w", s.contentPackPath, err)
	}
	defer zf.Close()

	if _, err := io.Copy(w, zf); err != nil {
		return fmt.Errorf("while copying from content pack member %v to response: %w", s.contentPackPath, err)
	}

	return nil
}

type goTemplateServable struct {
	sp       string
	template *template.Template
}

func (s *goTemplateServable) servingPath() string {
	return s.sp
}

func (s *goTemplateServable) serveHTTP(zr *zip.Reader, w http.ResponseWriter, r *http.Request) error {
	if err := s.template.Execute(w, nil); err != nil {
		return fmt.Errorf("while writing http response: %w", err)
	}
	return nil
}

type redirectServable struct {
	sp       string
	location string
}

func (s *redirectServable) servingPath() string {
	return s.sp
}

func (s *redirectServable) serveHTTP(zr *zip.Reader, w http.ResponseWriter, r *http.Request) error {
	http.Redirect(w, r, s.location, http.StatusFound)
	return nil
}

type Handler struct {
	zr *zip.Reader

	servables map[string]servable
}

func NewHandler(zr *zip.Reader) (*Handler, error) {
	// Load manifest from zip.
	mf, err := zr.Open("manifest")
	if err != nil {
		return nil, fmt.Errorf("while opening manifest: %w", err)
	}
	defer mf.Close()

	mb, err := io.ReadAll(mf)
	if err != nil {
		return nil, fmt.Errorf("while reading manifest: %w", err)
	}

	manifest := &manifestpb.Manifest{}
	if err := proto.Unmarshal(mb, manifest); err != nil {
		return nil, fmt.Errorf("while unmarshalling manifest: %w", err)
	}

	h := &Handler{
		zr:        zr,
		servables: map[string]servable{},
	}

	for _, servable := range manifest.Servables {
		switch e := servable.Entry.(type) {
		case *manifestpb.Servable_Static:
			sv := &staticServable{
				sp:              e.Static.ServingPath,
				contentPackPath: e.Static.ContentPackPath,
				mimeType:        e.Static.MimeType,
			}
			if err := h.registerServable(sv); err != nil {
				return nil, fmt.Errorf("while registering static: %w", err)
			}
		case *manifestpb.Servable_GoTemplate:
			if err := h.registerGoTemplateServable(e.GoTemplate); err != nil {
				return nil, fmt.Errorf("while registering go template: %w", err)
			}
		case *manifestpb.Servable_Redirect:
			if err := h.registerRedirectServable(e.Redirect); err != nil {
				return nil, fmt.Errorf("while registering redirect: %w", err)
			}
		default:
			return nil, fmt.Errorf("unknown type %T for servable", e)
		}
	}

	return h, nil
}

func (h *Handler) registerGoTemplateServable(gt *manifestpb.GoTemplate) error {
	baseTemplateReader, err := h.zr.Open(gt.BaseContentPackPath)
	if err != nil {
		return fmt.Errorf("while opening base template from content pack: %w", err)
	}
	defer baseTemplateReader.Close()

	baseTemplateBytes, err := io.ReadAll(baseTemplateReader)
	if err != nil {
		return fmt.Errorf("while reading base template from content pack: %w", err)
	}

	baseTemplate, err := template.New("").Parse(string(baseTemplateBytes))
	if err != nil {
		return fmt.Errorf("while parsing base template: %w", err)
	}

	specializationTemplateReader, err := h.zr.Open(gt.SpecializationContentPackPath)
	if err != nil {
		return fmt.Errorf("while opening specialization template from content pack: %w", err)
	}
	defer specializationTemplateReader.Close()

	specializationTemplateBytes, err := io.ReadAll(specializationTemplateReader)
	if err != nil {
		return fmt.Errorf("while reading specialization template from content pack: %w", err)
	}

	specializationTemplate, err := baseTemplate.Parse(string(specializationTemplateBytes))
	if err != nil {
		return fmt.Errorf("while parsing specialization template: %w", err)
	}

	sv := &goTemplateServable{
		sp:       gt.ServingPath,
		template: specializationTemplate,
	}
	if err := h.registerServable(sv); err != nil {
		return fmt.Errorf("while registering servable: %w", err)
	}

	return nil
}

func (h *Handler) registerRedirectServable(r *manifestpb.Redirect) error {
	sv := &redirectServable{
		sp:       r.ServingPath,
		location: r.Location,
	}
	if err := h.registerServable(sv); err != nil {
		return fmt.Errorf("while registering servable: %w", err)
	}
	return nil
}

func (h *Handler) registerServable(sv servable) error {
	_, found := h.servables[sv.servingPath()]
	if found {
		return fmt.Errorf("collision in manifest at %v", sv.servingPath())
	}

	h.servables[sv.servingPath()] = sv
	log.Printf("Registered servable at %v", sv.servingPath())

	return nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	servable, ok := h.servables[r.URL.Path]
	if !ok {
		log.Printf("Didn't find servable for %v", r.URL.Path)
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	if err := servable.serveHTTP(h.zr, w, r); err != nil {
		log.Printf("Error while executing servable for %v: %v", r.URL.Path, err)
		// Servable is required to write an error response.
		return
	}
}
