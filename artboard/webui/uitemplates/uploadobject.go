package uitemplates

import (
	"bytes"
	"fmt"
)

type UploadObjectParams struct {
	CollectionDisplayName string
	CollectionLink        string
	SelfLink              string

	// UploadLink is the target of the upload form.
	UploadLink string

	// Folder pre-fills the folder field.
	Folder string

	// UserError is set if the previous upload failed.
	UserError string

	// UploadedName is set to the name of the object that was just uploaded,
	// if any.
	UploadedName        string
	UploadedViewLink    string
	UploadedContentLink string
	UploadedIsImage     bool

	// CollectionID and FolderUploadLink configure the folder uploader, which
	// sends each file in a folder to FolderUploadLink.  MaxFileBytes is the
	// largest file it may send.
	CollectionID     string
	FolderUploadLink string
	MaxFileBytes     int64

	// ArchiveFiles is the number of files in a just-uploaded zip archive,
	// which is being extracted in the background.
	ArchiveFiles int
}

var uploadObjectText = `
{{define "title"}}Upload to {{.CollectionDisplayName}}{{end}}

{{define "breadcrumbs" -}}
  <li class="breadcrumb-item"><a href="/">Home</a></li>
  <li class="breadcrumb-item"><a href="{{.CollectionLink}}">{{.CollectionDisplayName}}</a></li>
  <li class="breadcrumb-item active" aria-current="page"><a href="{{.SelfLink}}">Upload</a></li>
{{- end}}

{{define "content"}}

<h1>Upload to {{.CollectionDisplayName}}</h1>

{{if .UserError}}
  <div class="alert alert-danger" role="alert">
    Error: {{.UserError}}
  </div>
{{end}}

{{if .UploadedName}}
  <div class="alert alert-success d-flex align-items-center gap-3" role="status">
    {{if .UploadedIsImage}}
      <a href="{{.UploadedViewLink}}"><img src="{{.UploadedContentLink}}" alt="{{.UploadedName}}" style="max-height: 6rem; max-width: 10rem"></a>
    {{end}}
    <div>Uploaded <a href="{{.UploadedViewLink}}" class="alert-link text-break">{{.UploadedName}}</a></div>
  </div>
{{end}}

{{if .ArchiveFiles}}
  <div class="alert alert-success" role="status">
    Received an archive of {{.ArchiveFiles}} file{{if ne .ArchiveFiles 1}}s{{end}}.  Its images and videos are being added to the collection in the background, which may take a while for a large archive.
    <a href="{{.CollectionLink}}" class="alert-link">Check the collection</a> to see them as they arrive.
  </div>
{{end}}

{{/* The folder field must come before the file: the server streams the
     body, and needs the folder before it reaches the file content. */}}
<form id="upload-form" method="POST" action="{{.UploadLink}}" enctype="multipart/form-data">
  <div class="mb-3">
    <label for="folder" class="form-label">Folder</label>
    <input id="folder"
           type="text"
           name="folder"
           value="{{.Folder}}"
           placeholder="optional/folder"
           class="form-control"
           aria-describedby="folder-help">
    <div id="folder-help" class="form-text">
      Uploads are numbered in order within this folder of the collection.
    </div>
  </div>

  <div id="drop-zone" class="mb-3 p-5 border border-2 rounded text-center text-body-secondary" style="border-style: dashed !important">
    <p class="mb-2">Paste an image anywhere on this page (Ctrl+V) to upload it immediately, or drop a file here.</p>
    <p class="mb-2 small">A .zip archive (up to 2 GiB) is extracted, and each image or video in it is uploaded under its path within the archive, inside the folder.  Files that already exist are left unchanged, so a failed archive upload can be retried.</p>
    <label for="file" class="visually-hidden">File</label>
    <input id="file" type="file" name="file" class="form-control mx-auto" style="max-width: 30rem">
  </div>

  <div id="upload-status" class="alert alert-info d-none" role="status"></div>

  <noscript><button type="submit" class="btn btn-primary">Upload</button></noscript>
</form>

<h2 class="h4 mt-4">Upload a folder</h2>
<div id="folder-uploader"
     data-endpoint="{{.FolderUploadLink}}"
     data-collection="{{.CollectionID}}"
     data-max-bytes="{{.MaxFileBytes}}">
  <noscript><p class="text-body-secondary">Uploading a folder requires JavaScript.</p></noscript>
</div>

{{end}}

{{define "scripts"}}
<script>
(function() {
  const form = document.getElementById("upload-form");
  const folder = document.getElementById("folder");
  const fileInput = document.getElementById("file");
  const dropZone = document.getElementById("drop-zone");
  const status = document.getElementById("upload-status");
  let submitting = false;

  // Uploads the first of files by submitting the form, which reloads the page
  // ready for the next upload.
  function upload(files) {
    if (submitting || !files || files.length === 0) {
      return;
    }
    submitting = true;
    const dt = new DataTransfer();
    dt.items.add(files[0]);
    fileInput.files = dt.files;
    folder.readOnly = true;
    status.textContent = "Uploading " + (files[0].name || "pasted data") + "…";
    status.classList.remove("d-none");
    form.submit();
  }

  document.addEventListener("paste", function(e) {
    // Ordinary text pastes (such as into the folder field) are left alone.
    if (e.clipboardData && e.clipboardData.files.length > 0) {
      e.preventDefault();
      upload(e.clipboardData.files);
    }
  });

  fileInput.addEventListener("change", function() {
    upload(fileInput.files);
  });

  dropZone.addEventListener("dragover", function(e) {
    e.preventDefault();
    dropZone.classList.add("bg-body-tertiary");
  });
  dropZone.addEventListener("dragleave", function() {
    dropZone.classList.remove("bg-body-tertiary");
  });
  dropZone.addEventListener("drop", function(e) {
    e.preventDefault();
    dropZone.classList.remove("bg-body-tertiary");
    upload(e.dataTransfer.files);
  });
})();
</script>
<script src="https://cdn.jsdelivr.net/npm/react@18.3.1/umd/react.production.min.js" integrity="sha384-DGyLxAyjq0f9SPpVevD6IgztCFlnMF6oW/XQGmfe+IsZ8TqEiDrcHkMLKI6fiB/Z" crossorigin="anonymous"></script>
<script src="https://cdn.jsdelivr.net/npm/react-dom@18.3.1/umd/react-dom.production.min.js" integrity="sha384-gTGxhz21lVGYNMcdJOyq01Edg0jhn/c22nsx0kyqP0TxaV5WVdsSH1fSDUf5YJj1" crossorigin="anonymous"></script>
<script src="/static/uploadfolder.js?v={{buildRevision}}"></script>
{{end}}
`

var uploadObjectTemplate = newPageTemplate(uploadObjectText)

func UploadObjectPage(params *UploadObjectParams) ([]byte, error) {
	b := bytes.Buffer{}
	if err := uploadObjectTemplate.Execute(&b, params); err != nil {
		return nil, fmt.Errorf("while executing template: %w", err)
	}
	return b.Bytes(), nil
}
