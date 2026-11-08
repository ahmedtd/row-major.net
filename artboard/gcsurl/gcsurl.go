// Package gcsurl parses Google Cloud Storage URLs of the form
// gs://bucket/optional/prefix.
package gcsurl

import (
	"errors"
	"regexp"
	"strings"
)

var (
	ErrNotGCSURL     = errors.New("URL must start with gs://")
	ErrInvalidBucket = errors.New("URL does not contain a valid bucket name")
)

// Bucket names: 3-222 characters of lowercase letters, digits, dashes,
// underscores, and dots, starting and ending with a letter or digit.
//
// https://cloud.google.com/storage/docs/buckets#naming
var bucketNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,220}[a-z0-9]$`)

// URL is a parsed GCS location.
type URL struct {
	Bucket string

	// Prefix is the object name prefix within the bucket.  It is either empty
	// (the whole bucket) or ends with "/".
	Prefix string
}

// Parse parses a gs://bucket/prefix URL.  A non-empty prefix is treated as a
// folder, so "gs://b/photos" and "gs://b/photos/" both yield Prefix "photos/".
func Parse(raw string) (*URL, error) {
	raw = strings.TrimSpace(raw)

	rest, ok := strings.CutPrefix(raw, "gs://")
	if !ok {
		return nil, ErrNotGCSURL
	}

	bucket, prefix, _ := strings.Cut(rest, "/")
	if !bucketNameRE.MatchString(bucket) {
		return nil, ErrInvalidBucket
	}

	prefix = strings.TrimLeft(prefix, "/")
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	return &URL{Bucket: bucket, Prefix: prefix}, nil
}

// String returns the canonical gs:// form of the URL.
func (u *URL) String() string {
	return "gs://" + u.Bucket + "/" + u.Prefix
}
