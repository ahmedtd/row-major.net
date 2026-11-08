package uitemplates

type HomeParams struct {
	LoggedIn    bool
	Collections []HomeCollection

	// ResyncRunning is set while an object metadata resync is in progress.
	ResyncRunning bool
}

type HomeCollection struct {
	DisplayName string
	StorageURL  string
	ShowLink    string
}

var homeText = `{{define "title"}}Home{{end}}
{{define "breadcrumbs" -}}
  <li class="breadcrumb-item active" aria-current="page"><a href="/">Home</a></li>
{{- end}}

{{define "content"}}
  <h1>Art Mood Board</h1>

  {{if .LoggedIn}}
    <h2 class="mt-4">Your Collections</h2>

    {{if .Collections}}
      <table class="table">
        <thead>
          <tr>
            <th scope="col">Name</th>
            <th scope="col">Location</th>
          </tr>
        </thead>
        <tbody class="table-group-divider">
        {{range .Collections}}
          <tr>
            <td><a href="{{.ShowLink}}">{{.DisplayName}}</a></td>
            <td><code>{{.StorageURL}}</code></td>
          </tr>
        {{end}}
        </tbody>
      </table>
    {{else}}
      <p>You don't have any collections yet.</p>
    {{end}}

    <p>
      <a href="/create-collection" class="btn btn-primary">Create Collection</a>
    </p>

    <h2 class="mt-4">Maintenance</h2>

    <form method="post" action="/resync-metadata">
      <p>
        Scan every collection's bucket and create metadata entries for objects
        that don't have one yet.  This runs in the background and reads every
        object, so it can take a while on large collections.
      </p>
      {{if .ResyncRunning}}
        <p>
          <button type="submit" class="btn btn-secondary" disabled>Resync in progress&hellip;</button>
        </p>
      {{else}}
        <p>
          <button type="submit" class="btn btn-secondary">Resync Object Metadata</button>
        </p>
      {{end}}
    </form>

    <p>
      <a href="/log-out">Log Out</a>
    </p>
  {{else}}
    <p>
      To get started, <a href="/log-in">Log In</a>.
    </p>
  {{end}}
{{end}}
`

var HomeTemplate = newPageTemplate(homeText)
