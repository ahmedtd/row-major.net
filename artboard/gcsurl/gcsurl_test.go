package gcsurl

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParse(t *testing.T) {
	testCases := []struct {
		in        string
		want      *URL
		wantErr   error
		wantCanon string
	}{
		{in: "gs://my-bucket", want: &URL{Bucket: "my-bucket"}, wantCanon: "gs://my-bucket/"},
		{in: "gs://my-bucket/", want: &URL{Bucket: "my-bucket"}, wantCanon: "gs://my-bucket/"},
		{in: "  gs://my-bucket/photos  ", want: &URL{Bucket: "my-bucket", Prefix: "photos/"}, wantCanon: "gs://my-bucket/photos/"},
		{in: "gs://my-bucket/photos/2024/", want: &URL{Bucket: "my-bucket", Prefix: "photos/2024/"}, wantCanon: "gs://my-bucket/photos/2024/"},
		{in: "gs://my.bucket_1//a", want: &URL{Bucket: "my.bucket_1", Prefix: "a/"}, wantCanon: "gs://my.bucket_1/a/"},
		{in: "https://storage.googleapis.com/my-bucket", wantErr: ErrNotGCSURL},
		{in: "my-bucket", wantErr: ErrNotGCSURL},
		{in: "", wantErr: ErrNotGCSURL},
		{in: "gs://", wantErr: ErrInvalidBucket},
		{in: "gs:///photos", wantErr: ErrInvalidBucket},
		{in: "gs://My-Bucket", wantErr: ErrInvalidBucket},
		{in: "gs://ab", wantErr: ErrInvalidBucket},
		{in: "gs://-bucket", wantErr: ErrInvalidBucket},
	}

	for _, tc := range testCases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := Parse(tc.in)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Parse(%q) err = %v, want %v", tc.in, err, tc.wantErr)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("Parse(%q) diff (-want +got):\n%s", tc.in, diff)
			}
			if got != nil && got.String() != tc.wantCanon {
				t.Errorf("Parse(%q).String() = %q, want %q", tc.in, got.String(), tc.wantCanon)
			}
		})
	}
}
