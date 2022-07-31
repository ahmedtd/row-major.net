package uitemplates

var baseText = `
<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8">
    <title>{{block "title" .}}Title{{end}} - MedTracker</title>
    <meta name="viewport" content="width=device-width, initial-scale=1.0">

    {{block "head" .}}{{end}}
    <style>
      #page-header {
          height: 100px;
      }

      #page-header-logo {
          display: inline-block;
          vertical-align: middle;
          height: 100%;
      }

      #page-header-title {
          display: inline;
          font-size: 28pt;
          vertical-align: middle;
      }

      .nav {
          display: block;
          width: 100%;
          left: 0px;
      }

      .breadcrumbs {
          display: inline-block;
          padding: 0px;
          margin: 0px;
          left: 0px;
          list-style: none;
      }

      .breadcrumbs li {
          display: inline;
          padding-right: 0.5em;
      }

      .breadcrumbs li:not(:first-child):before {
          content: '➛';
          display: inline-block;
          padding-right: 0.5em;
      }

      body {
          max-width: 55em;
          margin: auto;
          font-family: Sans-Serif;
          font-size: 10pt;
      }

      main h1 {
          font-size: 20pt;
      }

      main h2 {
          font-size: 14pt;
      }

      main figure {
          padding: 10px;
          margin-left: 0px;
          margin-right: 0px;
          background-color: bisque;
          max-width: 100%;
      }

      main figure * {
          max-width: 100%;
      }

      {{block "styles" .}}{{end}}
    </style>
  </head>
  <body>
    <header id="page-header"><img src=""
           id="page-header-logo"><h1 id="page-header-title">MedTracker</h1></header>

    <hr>
    <nav class="nav">{{block "breadcrumbs" .}}<nav class="nav"> <ul class="breadcrumbs"><li class="breadcrumbs-item"><a href="/">/root</a></li></ul> </nav>{{end}}</nav>
    <hr>

    <main>
      {{block "content" .}}{{end}}
    </main>

    <footer>
      <address>
	    <a href="mailto:admin@medtracker.dev">Contact</a>
      </address>
    </footer>

    {{block "scripts" .}}{{end}}
  </body>
</html>
`
