package uitemplates

import (
	"html/template"
	"runtime/debug"
)

// templateFuncs are the functions available to every page template.
var templateFuncs = template.FuncMap{
	"buildRevision": func() string { return revision },
}

var revision = buildRevision()

// newPageTemplate parses the base template followed by the page-specific
// template text.
func newPageTemplate(pageText string) *template.Template {
	return template.Must(template.Must(template.New("base").Funcs(templateFuncs).Parse(baseText)).Parse(pageText))
}

// buildRevision returns the VCS revision that the binary was built from, as
// stamped by the Go toolchain, with a "-dirty" suffix if the working tree had
// local modifications.
func buildRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	revision := "unknown"
	modified := false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}

	if modified {
		revision += "-dirty"
	}
	return revision
}

var baseText = `
<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{block "title" .}}Title{{end}} - Art Mood Board</title>
    <link href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.0-alpha1/dist/css/bootstrap.min.css" rel="stylesheet" integrity="sha384-GLhlTQ8iRABdZLl6O3oVMWSktQOp6b7In1Zl3/Jr59b6EGGoI1aFkw7cmDA6j6gD" crossorigin="anonymous">

    {{block "head" .}}{{end}}
  </head>
  <body>
    <div class="container">
      <nav class="navbar bg-body-tertiary">
        <div class="container-fluid">
          <a class="navbar-brand" href="/">Art Mood Board</a>
        </div>
      </nav>
  
      <nav aria-label="breadcrumb" class="border-bottom mt-3 mb-3">
        <ol class="breadcrumb">
          {{block "breadcrumbs" .}}<li class="breadcrumb-item active" aria-current="page">Home</li>{{end}}
        </ol>
      </nav>
  
      <main>
        {{block "content" .}}{{end}}
      </main>
  
      <footer class="pt-3 my-5 border-top">
        <address>
          Contact: admin@rumor-mill.dev
        </address>
        <p class="text-body-secondary small">Build: {{buildRevision}}</p>
      </footer>
    </div>

    <script src="https://cdn.jsdelivr.net/npm/bootstrap@5.3.0-alpha1/dist/js/bootstrap.bundle.min.js" integrity="sha384-w76AqPfDkMBDXo30jS1Sgez6pr3x5MlQ1ZAGC+nuZB+EYdgRZgiwxhTBTkF7CXvN" crossorigin="anonymous"></script>
    {{block "scripts" .}}{{end}}
  </body>
</html>
`
