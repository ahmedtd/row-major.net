package dblayer

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"row-major/artboard/dbtypes"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/firestore/apiv1/firestorepb"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type DB struct {
	firestoreClient *firestore.Client
}

func New(firestoreClient *firestore.Client) *DB {
	return &DB{
		firestoreClient: firestoreClient,
	}
}

var (
	ErrEmailMustNotBeEmpty        = errors.New("email must not be empty")
	ErrPasswordMustNotBeEmpty     = errors.New("password must not be empty")
	ErrUnknownUserOrWrongPassword = errors.New("unknown user or wrong password")
	ErrUserNotLoggedIn            = errors.New("user is not logged in")
	ErrPermissionDenied           = errors.New("permission denied")
)

// SessionFromPassword runs the password-based login process for a given user,
// returning a session ID or an error.
func (db *DB) SessionFromPassword(ctx context.Context, email, password string) (*dbtypes.Session, error) {
	if email == "" {
		return nil, ErrEmailMustNotBeEmpty
	}

	if password == "" {
		return nil, ErrPasswordMustNotBeEmpty
	}

	var userSnapshot *firestore.DocumentSnapshot
	userIter := db.firestoreClient.Collection("Users").Where("email", "==", email).Documents(ctx)
	defer userIter.Stop()
	for {
		var err error
		userSnapshot, err = userIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("while looking up user with email %q: %w", email, err)
		}

		// We only consider a single user.
		break
	}

	if userSnapshot == nil {
		return nil, ErrUnknownUserOrWrongPassword
	}

	user := &dbtypes.User{}
	if err := userSnapshot.DataTo(user); err != nil {
		return nil, fmt.Errorf("while unmarshaling user %q: %w", email, err)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrUnknownUserOrWrongPassword
	}

	sessionCookieBytes := make([]byte, 32)
	if _, err := rand.Read(sessionCookieBytes); err != nil {
		return nil, fmt.Errorf("while generating session cookie: %w", err)
	}

	sessionCookie := base64.StdEncoding.EncodeToString(sessionCookieBytes)

	expires := time.Now().Add(18 * time.Hour)

	sessions := db.firestoreClient.Collection("Sessions")
	session := &dbtypes.Session{
		Cookie:  sessionCookie,
		User:    userSnapshot.Ref,
		Expires: expires,
	}
	if _, _, err := sessions.Add(ctx, session); err != nil {
		return nil, fmt.Errorf("while storing session cookie: %w", err)
	}

	return session, nil
}

// DeleteSession deletes a session by its cookie.
func (db *DB) DeleteSession(ctx context.Context, cookie string) error {
	sessionIter := db.firestoreClient.Collection("Sessions").Where("cookie", "==", cookie).Documents(ctx)
	defer sessionIter.Stop()
	for {
		sessionSnapshot, err := sessionIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return fmt.Errorf("while looking up session: %w", err)
		}

		_, err = sessionSnapshot.Ref.Delete(ctx, firestore.LastUpdateTime(sessionSnapshot.UpdateTime))
		if err != nil {
			return fmt.Errorf("while deleting session: %w", err)
		}
	}

	return nil
}

// UserFromSessionCookie looks up a session from its cookie, and then returns
// the corresponding user.
func (db *DB) UserFromSessionCookie(ctx context.Context, cookie string) (*dbtypes.User, error) {
	var sessionSnapshot *firestore.DocumentSnapshot
	sessionIter := db.firestoreClient.Collection("Sessions").Where("cookie", "==", cookie).Documents(ctx)
	defer sessionIter.Stop()
	for {
		var err error
		sessionSnapshot, err = sessionIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("while looking up session: %w", err)
		}

		// We only consider a single session.
		break
	}
	if sessionSnapshot == nil {
		// Session object must have been cleaned up due to expiration; user is not logged in.
		slog.InfoContext(ctx, "No logged-in user because there was no session object corresponding to the cookie in the database.")
		return nil, nil
	}

	session := &dbtypes.Session{}
	if err := sessionSnapshot.DataTo(session); err != nil {
		return nil, fmt.Errorf("while unmarshaling session: %w", err)
	}

	if session.Expires.Before(time.Now()) {
		// Session object is expired; user is not logged in.
		slog.InfoContext(ctx, "No logged-in user because the session object in the database was expired.")
		return nil, nil
	}

	userSnapshot, err := session.User.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("while getting user linked from session: %w", err)
	}

	user := &dbtypes.User{}
	if err := userSnapshot.DataTo(user); err != nil {
		return nil, fmt.Errorf("while unmarshaling user: %w", err)
	}

	return user, nil
}

// CreateCollection stores a new collection, assigning its ID.
func (db *DB) CreateCollection(ctx context.Context, collection *dbtypes.Collection) error {
	ref := db.firestoreClient.Collection("Collections").NewDoc()
	collection.ID = ref.ID
	if _, err := ref.Create(ctx, collection); err != nil {
		return fmt.Errorf("while creating collection: %w", err)
	}
	return nil
}

// ListCollectionsForUser returns all collections that the user manages.
func (db *DB) ListCollectionsForUser(ctx context.Context, user *dbtypes.User) ([]*dbtypes.Collection, error) {
	iter := db.firestoreClient.Collection("Collections").Where("managingUsers", "array-contains", user.ID).Documents(ctx)
	defer iter.Stop()

	var collections []*dbtypes.Collection
	for {
		snap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("while iterating collections: %w", err)
		}

		collection := &dbtypes.Collection{}
		if err := snap.DataTo(collection); err != nil {
			return nil, fmt.Errorf("while unmarshaling collection %s: %w", snap.Ref.ID, err)
		}
		collections = append(collections, collection)
	}

	return collections, nil
}

// GetCollectionForUser retrieves a collection, returning ErrPermissionDenied if
// it does not exist or the user does not manage it.
func (db *DB) GetCollectionForUser(ctx context.Context, user *dbtypes.User, id string) (*dbtypes.Collection, error) {
	if id == "" {
		return nil, ErrPermissionDenied
	}

	snap, err := db.firestoreClient.Collection("Collections").Doc(id).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return nil, ErrPermissionDenied
	}
	if err != nil {
		return nil, fmt.Errorf("while retrieving collection %s: %w", id, err)
	}

	collection := &dbtypes.Collection{}
	if err := snap.DataTo(collection); err != nil {
		return nil, fmt.Errorf("while unmarshaling collection %s: %w", id, err)
	}

	if !slices.Contains(collection.ManagingUsers, user.ID) {
		return nil, ErrPermissionDenied
	}

	return collection, nil
}

// objectMetadataRef returns the document reference for an object's metadata.
//
// The document ID is derived from the collection ID and object path, so that
// lookups and upserts don't need a query, and concurrent first writes for the
// same object land on the same document.  It is hashed because object names
// can contain "/" and can be longer than Firestore allows for document IDs.
func (db *DB) objectMetadataRef(collectionID, path string) *firestore.DocumentRef {
	h := sha256.New()
	h.Write([]byte(collectionID))
	h.Write([]byte{0})
	h.Write([]byte(path))
	return db.firestoreClient.Collection("ObjectMetadata").Doc(hex.EncodeToString(h.Sum(nil)))
}

// GetObjectMetadata returns the metadata for an object in a collection.  If
// no metadata has been recorded for the object, it returns an empty
// ObjectMetadata rather than an error.
//
// The caller is responsible for checking that the user may access the
// collection.
func (db *DB) GetObjectMetadata(ctx context.Context, collectionID, path string) (*dbtypes.ObjectMetadata, error) {
	ref := db.objectMetadataRef(collectionID, path)
	snap, err := ref.Get(ctx)
	if status.Code(err) == codes.NotFound {
		return &dbtypes.ObjectMetadata{
			ID:         ref.ID,
			Collection: collectionID,
			Path:       path,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("while retrieving metadata for object %q in collection %s: %w", path, collectionID, err)
	}

	md := &dbtypes.ObjectMetadata{}
	if err := snap.DataTo(md); err != nil {
		return nil, fmt.Errorf("while unmarshaling metadata for object %q in collection %s: %w", path, collectionID, err)
	}
	return md, nil
}

// AddObjectTag associates tag with an object in a collection, creating the
// object's metadata entry if necessary.  Adding a tag that the object already
// has is a no-op.
//
// The caller is responsible for checking that the user may access the
// collection, and for validating the tag.
func (db *DB) AddObjectTag(ctx context.Context, collectionID, path, tag string) error {
	ref := db.objectMetadataRef(collectionID, path)
	// We don't know the caption here without a read, but the trigrams of the
	// path are always a subset of the correct set, so union them in.  This
	// makes an entry created by this call searchable by path.
	pathTrigrams := searchTrigrams(path, "")
	trigrams := make([]any, len(pathTrigrams))
	for i, t := range pathTrigrams {
		trigrams[i] = t
	}
	fields := map[string]any{
		"id":         ref.ID,
		"collection": collectionID,
		"path":       path,
		"tags":       firestore.ArrayUnion(tag),
	}
	if len(trigrams) > 0 {
		fields["trigrams"] = firestore.ArrayUnion(trigrams...)
	}
	_, err := ref.Set(ctx, fields, firestore.MergeAll)
	if err != nil {
		return fmt.Errorf("while adding tag to object %q in collection %s: %w", path, collectionID, err)
	}
	return nil
}

// SetObjectCaption sets the caption of an object in a collection, creating the
// object's metadata entry if necessary.  An empty caption clears it.
//
// The caller is responsible for checking that the user may access the
// collection, and for validating the caption.
func (db *DB) SetObjectCaption(ctx context.Context, collectionID, path, caption string) error {
	ref := db.objectMetadataRef(collectionID, path)
	_, err := ref.Set(ctx, map[string]any{
		"id":         ref.ID,
		"collection": collectionID,
		"path":       path,
		"caption":    caption,
		// Path and caption are both known here, so the trigrams can be
		// written in full, atomically with the caption.
		"trigrams": searchTrigrams(path, caption),
	}, firestore.MergeAll)
	if err != nil {
		return fmt.Errorf("while setting caption of object %q in collection %s: %w", path, collectionID, err)
	}
	return nil
}

// SearchObjects returns the metadata of objects in a collection that match a
// substring search query, ordered by path.
//
// The query is split on whitespace into terms, and an object matches if every
// term appears (ignoring case) as a substring of its path or its caption.
//
// Candidates are found using the trigrams field.  Firestore allows only one
// array-contains filter per query, so rather than running a query per trigram
// and intersecting the (unbounded) results, we count how many objects contain
// each of the query's trigrams, then stream the objects containing the rarest
// one in path order, checking each candidate in memory.  This keeps paging by
// path cheap: we stop reading as soon as we have limit matches.  If no term is
// long enough to have a trigram, every object in the collection is a
// candidate.
//
// Only objects whose path sorts strictly after `after` are returned, so that
// callers can page through results by passing the last path of the previous
// page.  Pass "" to start from the beginning.  At most limit results are
// returned.
//
// This relies on a composite index on ObjectMetadata of (collection ASC,
// trigrams CONTAINS, path ASC), and one of (collection ASC, path ASC).
//
// The caller is responsible for checking that the user may access the
// collection.
func (db *DB) SearchObjects(ctx context.Context, collectionID, query, after string, limit int) ([]*dbtypes.ObjectMetadata, error) {
	terms := parseSearchQuery(query)

	base := db.firestoreClient.Collection("ObjectMetadata").Where("collection", "==", collectionID)

	q := base
	if trigrams := queryTrigrams(terms); len(trigrams) > 0 {
		rarest, n, err := db.rarestTrigram(ctx, base, trigrams)
		if err != nil {
			return nil, fmt.Errorf("while searching objects in collection %s: %w", collectionID, err)
		}
		if n == 0 {
			// Some trigram of the query appears nowhere, so nothing can match.
			return nil, nil
		}
		q = q.Where("trigrams", "array-contains", rarest)
	}
	q = q.OrderBy("path", firestore.Asc)
	if after != "" {
		q = q.StartAfter(after)
	}

	iter := q.Documents(ctx)
	defer iter.Stop()

	var results []*dbtypes.ObjectMetadata
	for len(results) < limit {
		snap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("while searching objects in collection %s: %w", collectionID, err)
		}

		md := &dbtypes.ObjectMetadata{}
		if err := snap.DataTo(md); err != nil {
			return nil, fmt.Errorf("while unmarshaling object metadata in collection %s: %w", collectionID, err)
		}
		if !matchesAllTerms(md.Path, md.Caption, terms) {
			continue
		}
		results = append(results, md)
	}

	return results, nil
}

// rarestTrigram returns the trigram that the fewest documents matched by q
// contain, along with that count.  Counting is cheap in Firestore (billed per
// batch of index entries, not per document), so this is much less expensive
// than reading the documents for every trigram.
func (db *DB) rarestTrigram(ctx context.Context, q firestore.Query, trigrams []string) (string, int64, error) {
	counts := make([]int64, len(trigrams))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(16)
	for i, t := range trigrams {
		g.Go(func() error {
			tq := q.Where("trigrams", "array-contains", t)
			res, err := tq.NewAggregationQuery().WithCount("count").Get(gctx)
			if err != nil {
				return fmt.Errorf("while counting objects containing trigram %q: %w", t, err)
			}
			n, err := aggregateCount(res, "count")
			if err != nil {
				return fmt.Errorf("while counting objects containing trigram %q: %w", t, err)
			}
			counts[i] = n
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return "", 0, err
	}

	best := 0
	for i := range counts {
		if counts[i] < counts[best] {
			best = i
		}
	}
	return trigrams[best], counts[best], nil
}

// aggregateCount extracts a count from an aggregation result.
func aggregateCount(res firestore.AggregationResult, alias string) (int64, error) {
	v, ok := res[alias]
	if !ok {
		return 0, fmt.Errorf("aggregation result has no %q field", alias)
	}
	pv, ok := v.(*firestorepb.Value)
	if !ok {
		return 0, fmt.Errorf("aggregation result field %q has unexpected type %T", alias, v)
	}
	return pv.GetIntegerValue(), nil
}

// ListAllCollections returns every collection, regardless of which users
// manage it.  It is intended for background processing, not for serving user
// requests.
func (db *DB) ListAllCollections(ctx context.Context) ([]*dbtypes.Collection, error) {
	iter := db.firestoreClient.Collection("Collections").Documents(ctx)
	defer iter.Stop()

	var collections []*dbtypes.Collection
	for {
		snap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("while iterating collections: %w", err)
		}

		collection := &dbtypes.Collection{}
		if err := snap.DataTo(collection); err != nil {
			return nil, fmt.Errorf("while unmarshaling collection %s: %w", snap.Ref.ID, err)
		}
		collections = append(collections, collection)
	}

	return collections, nil
}

// EnsureObjectMetadata makes sure that each of the given objects in a
// collection has an up-to-date metadata entry.  It creates an empty entry (no
// tags or caption) for any object that doesn't have one, and recomputes the
// trigrams of any existing entry whose stored trigrams are missing or stale.
// Other fields of existing entries are left untouched.  It returns the number
// of entries created and the number updated.
func (db *DB) EnsureObjectMetadata(ctx context.Context, collectionID string, paths []string) (created, updated int, err error) {
	if len(paths) == 0 {
		return 0, 0, nil
	}

	refs := make([]*firestore.DocumentRef, len(paths))
	for i, path := range paths {
		refs[i] = db.objectMetadataRef(collectionID, path)
	}

	snaps, err := db.firestoreClient.GetAll(ctx, refs)
	if err != nil {
		return 0, 0, fmt.Errorf("while retrieving metadata for objects in collection %s: %w", collectionID, err)
	}

	for i, snap := range snaps {
		if snap.Exists() {
			n, err := db.refreshTrigrams(ctx, snap)
			updated += n
			if err != nil {
				return created, updated, fmt.Errorf("while updating trigrams for object %q in collection %s: %w", paths[i], collectionID, err)
			}
			continue
		}

		md := &dbtypes.ObjectMetadata{
			ID:         refs[i].ID,
			Collection: collectionID,
			Path:       paths[i],
			// Store an empty array rather than null, so that array
			// operations in queries behave uniformly.
			Tags:     []string{},
			Trigrams: searchTrigrams(paths[i], ""),
		}
		// Create (rather than Set) so that we never clobber an entry
		// written concurrently, for example by AddObjectTag.
		_, err := refs[i].Create(ctx, md)
		if status.Code(err) == codes.AlreadyExists {
			continue
		}
		if err != nil {
			return created, updated, fmt.Errorf("while creating metadata for object %q in collection %s: %w", paths[i], collectionID, err)
		}
		created++
	}

	return created, updated, nil
}

// refreshTrigrams recomputes the trigrams of an existing metadata entry and
// writes them if they differ from what is stored.  It returns 1 if it wrote
// the entry, and 0 otherwise.
//
// The write is conditioned on the entry not having changed since snap was
// read, so it can't clobber a concurrent caption change.  If it has changed,
// the concurrent writer is responsible for the trigrams, and this is a no-op.
func (db *DB) refreshTrigrams(ctx context.Context, snap *firestore.DocumentSnapshot) (int, error) {
	md := &dbtypes.ObjectMetadata{}
	if err := snap.DataTo(md); err != nil {
		return 0, fmt.Errorf("while unmarshaling metadata: %w", err)
	}

	want := searchTrigrams(md.Path, md.Caption)
	if md.Trigrams != nil && slices.Equal(md.Trigrams, want) {
		return 0, nil
	}

	_, err := snap.Ref.Update(ctx, []firestore.Update{
		{Path: "trigrams", Value: want},
	}, firestore.LastUpdateTime(snap.UpdateTime))
	if status.Code(err) == codes.FailedPrecondition {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return 1, nil
}
