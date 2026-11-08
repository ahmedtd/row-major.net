package webui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"row-major/artboard/dblayer"
	"row-major/artboard/dbtypes"
	"row-major/artboard/gcsurl"
	"row-major/artboard/metadatasync"
	"row-major/artboard/webui/uitemplates"

	"cloud.google.com/go/storage"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/iterator"
)

// objectsPageSize is the number of objects shown per page of a collection.
const objectsPageSize = 100

type WebUI struct {
	db *dblayer.DB

	// storageClient accesses collection buckets using the server's ambient
	// (Workload Identity) credentials.
	storageClient *storage.Client

	// syncer creates missing object metadata entries when a user asks for a
	// resync.
	syncer *metadatasync.Syncer
}

func New(db *dblayer.DB, storageClient *storage.Client) *WebUI {
	return &WebUI{
		db:            db,
		storageClient: storageClient,
		syncer:        metadatasync.New(db, storageClient),
	}
}

// Register adds the UI's handlers to m.  If host is non-empty, the handlers
// only match requests with that Host header.
func (u *WebUI) Register(m *http.ServeMux, host string) {
	m.HandleFunc("GET "+host+"/{$}", u.homeHandler)
	m.HandleFunc("GET "+host+"/log-in", u.logInGetHandler)
	m.HandleFunc("POST "+host+"/log-in", u.logInPostHandler)
	m.HandleFunc("GET "+host+"/log-out", u.logOutGetHandler)
	m.HandleFunc("POST "+host+"/log-out", u.logOutPostHandler)
	m.HandleFunc("GET "+host+"/create-collection", u.createCollectionGetHandler)
	m.HandleFunc("POST "+host+"/create-collection", u.createCollectionPostHandler)
	m.HandleFunc("GET "+host+"/show-collection", u.showCollectionHandler)
	m.HandleFunc("GET "+host+"/view-object", u.viewObjectHandler)
	m.HandleFunc("POST "+host+"/add-object-tag", u.addObjectTagHandler)
	m.HandleFunc("GET "+host+"/collection-object", u.objectContentHandler)
	m.HandleFunc("POST "+host+"/resync-metadata", u.resyncMetadataHandler)

	// Catch-all so that requests for this host never fall through to other
	// sites registered on the same mux.
	m.HandleFunc(host+"/", http.NotFound)
}

// getLoggedInUser loads the user associated with the session cookie in the
// request, if it exists.
func (u *WebUI) getLoggedInUser(ctx context.Context, r *http.Request) (string, *dbtypes.User, error) {
	var sessionCookie *http.Cookie
	for _, cookie := range r.Cookies() {
		if cookie.Name == "ArtBoard-Session" {
			sessionCookie = cookie
		}
	}
	if sessionCookie == nil {
		// No session cookie; user is not logged in.
		slog.InfoContext(ctx, "No logged-in user because there was no session cookie.")
		return "", nil, nil
	}

	user, err := u.db.UserFromSessionCookie(ctx, sessionCookie.Value)
	if err != nil {
		return "", nil, fmt.Errorf("while getting user from session cookie: %w", err)
	}
	if user == nil {
		// Session cookie didn't correspond to an active session.
		return "", nil, nil
	}

	slog.InfoContext(ctx, "Determined logged-in user",
		slog.Group("user",
			slog.String("id", user.ID),
			slog.String("email", user.Email),
		),
	)

	return sessionCookie.Value, user, nil
}

func (u *WebUI) checkSession(ctx context.Context, w http.ResponseWriter, r *http.Request, redirectAfterLogin string) *dbtypes.User {
	var sessionCookie *http.Cookie
	for _, cookie := range r.Cookies() {
		if cookie.Name == "ArtBoard-Session" {
			sessionCookie = cookie
		}
	}
	if sessionCookie == nil {
		// User is not logged in.  Send them to log in.
		slog.InfoContext(ctx, "No logged-in user because there was no session cookie.  Redirecting to login.")
		http.Redirect(w, r, logInLink("", redirectAfterLogin), http.StatusFound)
		return nil
	}

	user, err := u.db.UserFromSessionCookie(ctx, sessionCookie.Value)
	if err != nil {
		slog.ErrorContext(ctx, "Error while validating session cookie", slog.Any("err", err))
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return nil
	}
	if user == nil {
		// User is not logged in.  For example, there was a session cookie, but
		// it corresponds to an expired session.
		slog.InfoContext(ctx, "Session cookie didn't correspond to an active session.")
		http.Redirect(w, r, logInLink("", redirectAfterLogin), http.StatusFound)
		return nil
	}

	slog.InfoContext(ctx, "Determined logged-in user",
		slog.Group("user",
			slog.String("id", user.ID),
			slog.String("email", user.Email),
		),
	)

	return user
}

// homeHandler renders the home page.
func (u *WebUI) homeHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	params := &uitemplates.HomeParams{}

	_, user, err := u.getLoggedInUser(r.Context(), r)
	if err != nil {
		slog.ErrorContext(ctx, "Error while getting logged-in user", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}
	if user != nil {
		params.LoggedIn = true
		params.ResyncRunning = u.syncer.Running()

		collections, err := u.db.ListCollectionsForUser(ctx, user)
		if err != nil {
			slog.ErrorContext(ctx, "Error while listing collections", slog.Any("err", err))
			http.Error(w, "Internal Error", http.StatusInternalServerError)
			return
		}
		for _, c := range collections {
			params.Collections = append(params.Collections, uitemplates.HomeCollection{
				DisplayName: c.DisplayName,
				StorageURL:  c.StorageURL,
				ShowLink:    showCollectionLink(c.ID, ""),
			})
		}
	}

	content := bytes.Buffer{}
	if err := uitemplates.HomeTemplate.Execute(&content, params); err != nil {
		slog.ErrorContext(ctx, "Error while executing template", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if _, err := io.Copy(w, &content); err != nil {
		// It's too late to write an error to the HTTP response.
		slog.ErrorContext(ctx, "Error while writing output", slog.Any("err", err))
		return
	}
}

// resyncMetadataHandler starts a background pass over all collections that
// creates any missing object metadata entries, then sends the user back to the
// home page.  It doesn't wait for the pass to finish.
//
// Any logged-in user may trigger a resync.
func (u *WebUI) resyncMetadataHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	user := u.checkSession(ctx, w, r, "/")
	if user == nil {
		// checkSession already wrote an error or redirect.
		return
	}

	if u.syncer.Start() {
		slog.InfoContext(ctx, "User started object metadata resync", slog.String("user", user.ID))
	} else {
		slog.InfoContext(ctx, "Object metadata resync already in progress", slog.String("user", user.ID))
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

func logInLink(userError, redirectTarget string) string {
	q := url.Values{}
	if userError != "" {
		q.Add("user-error", userError)
	}
	if redirectTarget != "" {
		q.Add("redirect-target", redirectTarget)
	}
	link := &url.URL{
		Path:     "/log-in",
		RawQuery: q.Encode(),
	}
	return link.String()
}

func (u *WebUI) logInGetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	_, user, err := u.getLoggedInUser(ctx, r)
	if err != nil {
		slog.ErrorContext(ctx, "Error while getting logged-in user", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if err := r.ParseForm(); err != nil {
		slog.ErrorContext(ctx, "Error while parsing form", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if user != nil {
		// User is already logged in.
		if target := r.Form.Get("redirect-target"); target != "" {
			http.Redirect(w, r, target, http.StatusFound)
			return
		}
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	params := &uitemplates.LogInParams{
		UserError: r.Form.Get("user-error"),
	}

	content := bytes.Buffer{}
	if err := uitemplates.LogInTemplate.Execute(&content, params); err != nil {
		slog.ErrorContext(ctx, "Error while executing template", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if _, err := io.Copy(w, &content); err != nil {
		// It's too late to write an error to the HTTP response.
		slog.ErrorContext(ctx, "Error while writing output", slog.Any("err", err))
		return
	}
}

func (u *WebUI) logInPostHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	_, user, err := u.getLoggedInUser(ctx, r)
	if err != nil {
		slog.ErrorContext(ctx, "Error while getting logged-in user", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if err := r.ParseForm(); err != nil {
		slog.ErrorContext(ctx, "Error while parsing form", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if user != nil {
		// User is already logged in.
		if target := r.Form.Get("redirect-target"); target != "" {
			http.Redirect(w, r, target, http.StatusFound)
			return
		}
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	session, err := u.db.SessionFromPassword(ctx, r.PostForm.Get("email"), r.PostForm.Get("password"))
	if err == dblayer.ErrEmailMustNotBeEmpty {
		http.Redirect(w, r, logInLink("Email must not be empty", r.Form.Get("redirect-target")), http.StatusFound)
		return
	}
	if err == dblayer.ErrPasswordMustNotBeEmpty {
		http.Redirect(w, r, logInLink("Password must not be empty", r.Form.Get("redirect-target")), http.StatusFound)
		return
	}
	if err == dblayer.ErrUnknownUserOrWrongPassword {
		http.Redirect(w, r, logInLink("Unknown user or wrong password", r.Form.Get("redirect-target")), http.StatusFound)
		return
	}
	if err != nil {
		slog.ErrorContext(ctx, "Error while processing log in form", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	cookie := &http.Cookie{
		Name:     "ArtBoard-Session",
		Value:    session.Cookie,
		SameSite: http.SameSiteStrictMode,
		Expires:  session.Expires,
	}

	// User successfully logged in
	http.SetCookie(w, cookie)

	target := r.Form.Get("redirect-target")
	if target == "" {
		target = "/"
	}

	http.Redirect(w, r, target, http.StatusFound)
}

func (u *WebUI) logOutGetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	_, user, err := u.getLoggedInUser(ctx, r)
	if err != nil {
		slog.ErrorContext(ctx, "Error while getting logged-in user", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if user == nil {
		// User is already logged out?
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	content, err := uitemplates.LogOutPage(&uitemplates.LogOutParams{})
	if err != nil {
		slog.ErrorContext(ctx, "Error while executing template", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if _, err := w.Write(content); err != nil {
		// It's too late to write an error to the HTTP response.
		slog.ErrorContext(ctx, "Error while writing output", slog.Any("err", err))
		return
	}
}

func (u *WebUI) logOutPostHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	cookie, user, err := u.getLoggedInUser(ctx, r)
	if err != nil {
		slog.ErrorContext(ctx, "Error while getting logged-in user", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if user == nil {
		// User is already logged out.
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	err = u.db.DeleteSession(ctx, cookie)
	if err != nil {
		slog.ErrorContext(ctx, "Error while deleting session", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:   "ArtBoard-Session",
		MaxAge: -1,
	})

	http.Redirect(w, r, "/", http.StatusFound)
}

func createCollectionLink(userError, displayName, storageURL string) string {
	q := url.Values{}
	if userError != "" {
		q.Add("user-error", userError)
	}
	if displayName != "" {
		q.Add("name", displayName)
	}
	if storageURL != "" {
		q.Add("storage-url", storageURL)
	}
	link := &url.URL{
		Path:     "/create-collection",
		RawQuery: q.Encode(),
	}
	return link.String()
}

func (u *WebUI) createCollectionGetHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		slog.ErrorContext(ctx, "Error while parsing form", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	user := u.checkSession(ctx, w, r, createCollectionLink("", r.Form.Get("name"), r.Form.Get("storage-url")))
	if user == nil {
		// checkSession already wrote an error or redirect.
		return
	}
	// No permissions check necessary; any user may create a collection.

	params := &uitemplates.CreateCollectionParams{
		UserError:   r.Form.Get("user-error"),
		DisplayName: r.Form.Get("name"),
		StorageURL:  r.Form.Get("storage-url"),
	}
	content, err := uitemplates.CreateCollectionPage(params)
	if err != nil {
		slog.ErrorContext(ctx, "Error while executing template", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if _, err := w.Write(content); err != nil {
		// It's too late to write an error to the HTTP response.
		slog.ErrorContext(ctx, "Error while writing output", slog.Any("err", err))
		return
	}
}

func (u *WebUI) createCollectionPostHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		slog.ErrorContext(ctx, "Error while parsing form", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	displayName := strings.TrimSpace(r.PostForm.Get("name"))
	rawStorageURL := strings.TrimSpace(r.PostForm.Get("storage-url"))

	user := u.checkSession(ctx, w, r, createCollectionLink("", displayName, rawStorageURL))
	if user == nil {
		// checkSession already wrote an error or redirect.
		return
	}
	// No permissions check necessary; any user may create a collection.

	if displayName == "" {
		http.Redirect(w, r, createCollectionLink("Name must not be empty", displayName, rawStorageURL), http.StatusFound)
		return
	}

	storageURL, err := gcsurl.Parse(rawStorageURL)
	if err != nil {
		http.Redirect(w, r, createCollectionLink("Invalid Cloud Storage URL: "+err.Error(), displayName, rawStorageURL), http.StatusFound)
		return
	}

	// Make sure we can actually read the location before saving it, so that
	// the user gets immediate feedback about missing permissions or typos.
	if err := u.checkBucketReadable(ctx, storageURL); err != nil {
		slog.InfoContext(ctx, "Cloud Storage location is not readable", slog.String("storageURL", storageURL.String()), slog.Any("err", err))
		userError := fmt.Sprintf("Could not list objects in %s.  Check that the bucket exists and that this site has been granted read access to it.", storageURL)
		http.Redirect(w, r, createCollectionLink(userError, displayName, rawStorageURL), http.StatusFound)
		return
	}

	collection := &dbtypes.Collection{
		DisplayName:   displayName,
		ManagingUsers: []string{user.ID},
		StorageURL:    storageURL.String(),
	}
	if err := u.db.CreateCollection(ctx, collection); err != nil {
		slog.ErrorContext(ctx, "Error while creating collection", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, showCollectionLink(collection.ID, ""), http.StatusFound)
}

// checkBucketReadable verifies that the server can list objects at loc.
func (u *WebUI) checkBucketReadable(ctx context.Context, loc *gcsurl.URL) error {
	it := u.storageClient.Bucket(loc.Bucket).Objects(ctx, &storage.Query{Prefix: loc.Prefix})
	_, err := it.Next()
	if err != nil && err != iterator.Done {
		return err
	}
	return nil
}

func showCollectionLink(id, pageToken string) string {
	q := url.Values{}
	q.Add("id", id)
	if pageToken != "" {
		q.Add("page-token", pageToken)
	}
	link := &url.URL{
		Path:     "/show-collection",
		RawQuery: q.Encode(),
	}
	return link.String()
}

// showCollectionSearchLink links to the objects in a collection that have a
// tag containing query.  If after is non-empty, only objects whose names sort
// after it are shown.
func showCollectionSearchLink(id, query, after string) string {
	q := url.Values{}
	q.Add("id", id)
	q.Add("q", query)
	if after != "" {
		q.Add("after", after)
	}
	link := &url.URL{
		Path:     "/show-collection",
		RawQuery: q.Encode(),
	}
	return link.String()
}

func (u *WebUI) showCollectionHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		slog.ErrorContext(ctx, "Error while parsing form", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	collectionID := r.Form.Get("id")
	pageToken := r.Form.Get("page-token")
	searchQuery := strings.TrimSpace(r.Form.Get("q"))
	after := r.Form.Get("after")

	redirectAfterLogin := showCollectionLink(collectionID, pageToken)
	if searchQuery != "" {
		redirectAfterLogin = showCollectionSearchLink(collectionID, searchQuery, after)
	}
	user := u.checkSession(ctx, w, r, redirectAfterLogin)
	if user == nil {
		// checkSession already wrote an error or redirect.
		return
	}

	collection, loc := u.loadCollection(w, r, user, collectionID)
	if collection == nil {
		// loadCollection already wrote an error.
		return
	}

	params := &uitemplates.ShowCollectionParams{
		DisplayName:  collection.DisplayName,
		StorageURL:   collection.StorageURL,
		SelfLink:     showCollectionLink(collection.ID, ""),
		CollectionID: collection.ID,
		SearchLink:   "/show-collection",
		SearchQuery:  searchQuery,
	}

	if searchQuery != "" {
		objects, nextAfter, err := u.searchCollectionObjects(ctx, collection, loc, searchQuery, after)
		if err != nil {
			slog.ErrorContext(ctx, "Error while searching collection objects", slog.String("collection", collection.ID), slog.String("query", searchQuery), slog.Any("err", err))
			params.UserError = "Could not search the collection."
		} else {
			params.Objects = objects
			if nextAfter != "" {
				params.NextPageLink = showCollectionSearchLink(collection.ID, searchQuery, nextAfter)
			}
		}
	} else {
		objects, nextPageToken, err := u.listCollectionObjects(ctx, collection, loc, pageToken)
		if err != nil {
			slog.ErrorContext(ctx, "Error while listing collection objects", slog.String("collection", collection.ID), slog.Any("err", err))
			params.UserError = "Could not list objects in the collection's bucket.  Check that the bucket exists and that this site has read access to it."
		} else {
			params.Objects = objects
			if nextPageToken != "" {
				params.NextPageLink = showCollectionLink(collection.ID, nextPageToken)
			}
		}
	}

	content, err := uitemplates.ShowCollectionPage(params)
	if err != nil {
		slog.ErrorContext(ctx, "Error while executing template", slog.Any("err", err))
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}

	if _, err := w.Write(content); err != nil {
		// It's too late to write an error to the HTTP response.
		slog.ErrorContext(ctx, "Error while writing output", slog.Any("err", err))
		return
	}
}

func showCollectionObject(collectionID, name string, obj *storage.ObjectAttrs) uitemplates.ShowCollectionObject {
	return uitemplates.ShowCollectionObject{
		Name:        name,
		ViewLink:    viewObjectLink(collectionID, name),
		ContentType: obj.ContentType,
		Size:        humanBytes(obj.Size),
		Updated:     obj.Updated.UTC().Format(time.DateTime),
	}
}

// listCollectionObjects lists one page of the objects in a collection's bucket,
// returning the token for the next page, or "" if there are no more.
func (u *WebUI) listCollectionObjects(ctx context.Context, collection *dbtypes.Collection, loc *gcsurl.URL, pageToken string) ([]uitemplates.ShowCollectionObject, string, error) {
	it := u.storageClient.Bucket(loc.Bucket).Objects(ctx, &storage.Query{Prefix: loc.Prefix})
	var objs []*storage.ObjectAttrs
	nextPageToken, err := iterator.NewPager(it, objectsPageSize, pageToken).NextPage(&objs)
	if err != nil {
		return nil, "", err
	}

	var objects []uitemplates.ShowCollectionObject
	for _, obj := range objs {
		name := strings.TrimPrefix(obj.Name, loc.Prefix)
		if !validObjectName(name) {
			// Skip folder placeholder objects.
			continue
		}
		objects = append(objects, showCollectionObject(collection.ID, name, obj))
	}
	return objects, nextPageToken, nil
}

// searchCollectionObjects returns one page of the objects in a collection that
// have a tag containing query, starting after the object named after.  It also
// returns the name to pass as after to get the next page, or "" if there are
// no more.
func (u *WebUI) searchCollectionObjects(ctx context.Context, collection *dbtypes.Collection, loc *gcsurl.URL, query, after string) ([]uitemplates.ShowCollectionObject, string, error) {
	// Fetch one extra result so we know whether there is a next page.
	mds, err := u.db.SearchObjectsByTag(ctx, collection.ID, query, after, objectsPageSize+1)
	if err != nil {
		return nil, "", err
	}
	nextAfter := ""
	if len(mds) > objectsPageSize {
		mds = mds[:objectsPageSize]
		nextAfter = mds[len(mds)-1].Path
	}

	// Look up the current attributes of each matching object, so the results
	// look like a normal listing.
	bucket := u.storageClient.Bucket(loc.Bucket)
	attrs := make([]*storage.ObjectAttrs, len(mds))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(16)
	for i, md := range mds {
		if !validObjectName(md.Path) {
			continue
		}
		g.Go(func() error {
			a, err := bucket.Object(loc.Prefix + md.Path).Attrs(gctx)
			if errors.Is(err, storage.ErrObjectNotExist) {
				// The object was deleted after it was tagged.
				return nil
			}
			if err != nil {
				return fmt.Errorf("while getting attributes of object %q: %w", md.Path, err)
			}
			attrs[i] = a
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, "", err
	}

	var objects []uitemplates.ShowCollectionObject
	for i, md := range mds {
		if attrs[i] == nil {
			continue
		}
		objects = append(objects, showCollectionObject(collection.ID, md.Path, attrs[i]))
	}
	return objects, nextAfter, nil
}

// humanBytes formats a byte count using binary (KiB, MiB, ...) units.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
