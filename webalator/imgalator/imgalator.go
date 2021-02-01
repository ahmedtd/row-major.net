package imgalator

import (
	"context"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
)

type Site struct {
	pathPrefix string
	bucketName string

	gcsClient *storage.Client
}

func New(ctx context.Context, pathPrefix, bucketName string) (*Site, error) {
	if len(pathPrefix) > 0 && pathPrefix[len(pathPrefix)-1] == '/' {
		return nil, fmt.Errorf("pathPrefix should not end in /")
	}

	s := &Site{
		pathPrefix: pathPrefix,
		bucketName: bucketName,
	}

	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("while creating GCS client: %w", err)
	}
	s.gcsClient = gcsClient

	return s, nil
}

func (s *Site) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("imgalator: handling path=%q", r.URL)

	path := r.URL.EscapedPath()

	if ok, redir := s.matchListBuckets(path); ok {
		if redir {
			http.Redirect(w, r, s.formListBuckets(), http.StatusFound)
			return
		}
		s.handlerListBuckets(w, r)
		return
	}

	if ok, redir, bucket := s.matchListObjects(path); ok {
		if redir {
			http.Redirect(w, r, s.formListObjects(bucket), http.StatusFound)
			return
		}
		s.handlerListObjects(bucket, w, r)
		return
	}

	if ok, redir, bucket, object := s.matchGetObject(path); ok {
		if redir {
			http.Redirect(w, r, s.formGetObject(bucket, object), http.StatusFound)
			return
		}
		s.handlerGetObject(bucket, object, w, r)
		return
	}

	http.Error(w, "Not found", http.StatusNotFound)
}

const listBucketsText = `<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8">
    <title>Imgalator: List Buckets</title>
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
  </head>
  <body>
    <ul>
    {{range .Buckets}}
      <li><a href="{{.URL}}">{{.Name}}</a></li>
    {{else}}
      No buckets to list!
    {{end}}
    </ul>
  </body>
</html>
`

var listBucketsTemplate = template.Must(template.New("buckets").Parse(listBucketsText))

func (s *Site) matchListBuckets(path string) (match, redir bool) {
	if path == s.pathPrefix+"/buckets/" {
		return true, false
	}
	if path == s.pathPrefix+"/buckets" {
		return true, true
	}
	return false, false
}

func (s *Site) formListBuckets() string {
	return s.pathPrefix + "/buckets/"
}

func (s *Site) handlerListBuckets(w http.ResponseWriter, r *http.Request) {
	type dataBucket struct {
		Name string
		URL  template.URL
	}

	data := struct {
		Buckets []dataBucket
	}{
		Buckets: []dataBucket{{
			Name: s.bucketName,
			URL:  template.URL(s.formListObjects(s.bucketName)),
		}},
	}

	err := listBucketsTemplate.Execute(w, data)
	if err != nil {
		log.Printf("Error while writing http response: %v", err)
		return
	}
}

const listObjectsText = `<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8">
    <title>Imgalator: List Objects</title>
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
  </head>
  <body>
    <ul>
    {{range .Objects}}
      <li><a href="{{.URL}}">{{.Name}}</a></li>
    {{else}}
      No objects in this bucket!
    {{end}}
    </ul>
  </body>
</html>
`

var listObjectsTemplate = template.Must(template.New("bucket").Parse(listObjectsText))

func (s *Site) matchListObjects(path string) (match, redir bool, bucket string) {
	if !strings.HasPrefix(path, s.pathPrefix) {
		return false, false, ""
	}
	path = strings.TrimPrefix(path, s.pathPrefix)

	if !strings.HasPrefix(path, "/") {
		return false, false, ""
	}
	path = strings.TrimPrefix(path, "/")

	if !strings.HasPrefix(path, "buckets/") {
		return false, false, ""
	}
	path = strings.TrimPrefix(path, "buckets/")

	nextSlash := strings.Index(path, "/")
	if nextSlash == -1 {
		return false, false, ""
	}
	bucket = path[:nextSlash]
	path = path[nextSlash:]

	if !strings.HasPrefix(path, "/") {
		return false, false, ""
	}
	path = strings.TrimPrefix(path, "/")

	if !strings.HasPrefix(path, "objects") {
		return false, false, ""
	}
	path = strings.TrimPrefix(path, "objects")

	if path == "" {
		return true, true, bucket
	}

	if path == "/" {
		return true, false, bucket
	}

	return false, false, ""
}

func (s *Site) formListObjects(bucket string) string {
	return s.pathPrefix + "/buckets/" + bucket + "/objects/"
}

func (s *Site) handlerListObjects(bucketName string, w http.ResponseWriter, r *http.Request) {
	bkt := s.gcsClient.Bucket(bucketName)

	type listObject struct {
		Name string
		URL  template.URL
	}
	objects := []listObject{}

	objIt := bkt.Objects(r.Context(), nil)
	for {
		objAttr, err := objIt.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		objects = append(objects, listObject{
			Name: objAttr.Name,
			URL:  template.URL("https://storage.cloud.google.com/" + url.PathEscape(bucketName) + "/" + url.PathEscape(objAttr.Name)),
			// URL:  template.URL(path.Join(s.pathPrefix, "buckets", s.bucketName, "objects", objAttr.Name)),
		})
	}

	data := struct {
		BucketName string
		Objects    []listObject
	}{
		BucketName: s.bucketName,
		Objects:    objects,
	}

	err := listObjectsTemplate.Execute(w, data)
	if err != nil {
		log.Printf("Error while writing http response: %v", err)
		return
	}
}

func (s *Site) matchGetObject(path string) (match, redir bool, bucket, object string) {
	if !strings.HasPrefix(path, s.pathPrefix) {
		return false, false, "", ""
	}
	path = strings.TrimPrefix(path, s.pathPrefix)

	if !strings.HasPrefix(path, "/") {
		return false, false, "", ""
	}
	path = strings.TrimPrefix(path, "/")

	if !strings.HasPrefix(path, "buckets/") {
		return false, false, "", ""
	}
	path = strings.TrimPrefix(path, "buckets/")

	nextSlash := strings.Index(path, "/")
	if nextSlash == -1 {
		return false, false, "", ""
	}
	bucket = path[:nextSlash]
	path = path[nextSlash:]

	if !strings.HasPrefix(path, "/") {
		return false, false, "", ""
	}
	path = strings.TrimPrefix(path, "/")

	if !strings.HasPrefix(path, "objects") {
		return false, false, "", ""
	}
	path = strings.TrimPrefix(path, "objects")

	if !strings.HasPrefix(path, "/") {
		return false, false, "", ""
	}
	path = strings.TrimPrefix(path, "/")

	nextSlash = strings.Index(path, "/")
	if nextSlash != -1 {
		return false, false, "", ""
	}
	object, err := url.PathUnescape(path)
	if err != nil {
		return false, false, "", ""
	}

	return true, false, bucket, object
}

func (s *Site) formGetObject(bucket, object string) string {
	return s.pathPrefix + "/buckets/" + bucket + "/objects/" + object
}

func (s *Site) handlerGetObject(bucket, object string, w http.ResponseWriter, r *http.Request) {
	bkt := s.gcsClient.Bucket(bucket)

	reader, err := bkt.Object(object).NewReader(r.Context())
	if err != nil {
		log.Printf("imgalator: error while creating reader for %q: %v", object, err)
		http.Error(w, "Unknown", http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	_, err = io.Copy(w, reader)
	if err != nil {
		log.Printf("imgalator: error while writing response body: %v", err)
		return
	}
}
