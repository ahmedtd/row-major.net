package uitemplates

import (
	"bytes"
	"strings"
	"testing"
)

func TestHomeTemplate(t *testing.T) {
	b := bytes.Buffer{}
	err := HomeTemplate.Execute(&b, &HomeParams{
		LoggedIn: true,
		Collections: []HomeCollection{
			{DisplayName: "Sketches <b>", StorageURL: "gs://bucket/sketches/", ShowLink: "/show-collection?id=abc"},
		},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	for _, want := range []string{"Sketches &lt;b&gt;", "gs://bucket/sketches/", `href="/show-collection?id=abc"`} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestCreateCollectionPage(t *testing.T) {
	out, err := CreateCollectionPage(&CreateCollectionParams{UserError: "bad url", DisplayName: "x", StorageURL: "gs://y"})
	if err != nil {
		t.Fatalf("CreateCollectionPage: %v", err)
	}
	if !strings.Contains(string(out), "bad url") {
		t.Errorf("output missing user error")
	}
}

func TestShowCollectionPage(t *testing.T) {
	out, err := ShowCollectionPage(&ShowCollectionParams{
		DisplayName:  "Sketches",
		StorageURL:   "gs://bucket/sketches/",
		SelfLink:     "/show-collection?id=abc",
		Objects:      []ShowCollectionObject{{Name: "cat.png", ContentType: "image/png", Size: "1.0 KiB", Updated: "2026-01-01 00:00:00"}},
		NextPageLink: "/show-collection?id=abc&page-token=t",
	})
	if err != nil {
		t.Fatalf("ShowCollectionPage: %v", err)
	}
	for _, want := range []string{"cat.png", "image/png", "Next page"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("output missing %q", want)
		}
	}
}
