package uitemplates

var baseText = `
<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{block "title" .}}Title{{end}} - MedTracker</title>
    <link href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.0-alpha1/dist/css/bootstrap.min.css" rel="stylesheet" integrity="sha384-GLhlTQ8iRABdZLl6O3oVMWSktQOp6b7In1Zl3/Jr59b6EGGoI1aFkw7cmDA6j6gD" crossorigin="anonymous">

    {{block "head" .}}{{end}}
  </head>
  <body>
    <div class="container">
      <nav class="navbar bg-body-tertiary">
        <div class="container-fluid">
          <a class="navbar-brand" href="/">MedTracker</a>
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
	      <a href="mailto:admin@medtracker.dev">Contact</a>
        </address>
      </footer>
    </div>

    <script src="https://cdn.jsdelivr.net/npm/bootstrap@5.3.0-alpha1/dist/js/bootstrap.bundle.min.js" integrity="sha384-w76AqPfDkMBDXo30jS1Sgez6pr3x5MlQ1ZAGC+nuZB+EYdgRZgiwxhTBTkF7CXvN" crossorigin="anonymous"></script>
    {{block "scripts" .}}{{end}}
  </body>
</html>
`
