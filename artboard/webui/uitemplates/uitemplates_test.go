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
	for _, want := range []string{"Sketches &lt;b&gt;", "gs://bucket/sketches/", `href="/show-collection?id=abc"`, `action="/resync-metadata"`, "Resync Object Metadata"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestHomeTemplateResyncRunning(t *testing.T) {
	b := bytes.Buffer{}
	if err := HomeTemplate.Execute(&b, &HomeParams{LoggedIn: true, ResyncRunning: true}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(b.String(), "Resync in progress") {
		t.Errorf("output missing in-progress indicator")
	}
	if strings.Contains(b.String(), "Resync Object Metadata") {
		t.Errorf("output shows resync button while a resync is running")
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

func TestShowCollectionPageSearch(t *testing.T) {
	params := &ShowCollectionParams{
		DisplayName:  "Sketches",
		SelfLink:     "/show-collection?id=abc",
		CollectionID: "abc",
		SearchLink:   "/show-collection",
		SearchQuery:  "cat <b>",
	}
	out, err := ShowCollectionPage(params)
	if err != nil {
		t.Fatalf("ShowCollectionPage: %v", err)
	}
	for _, want := range []string{
		`action="/show-collection"`,
		`name="id" value="abc"`,
		`name="q" value="cat &lt;b&gt;"`,
		"No objects have a tag containing &ldquo;cat &lt;b&gt;&rdquo;.",
		"Clear",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("output missing %q", want)
		}
	}

	params.SearchQuery = ""
	out, err = ShowCollectionPage(params)
	if err != nil {
		t.Fatalf("ShowCollectionPage: %v", err)
	}
	if strings.Contains(string(out), "Clear") {
		t.Errorf("output has Clear button without an active search")
	}
}

func TestViewObjectPage(t *testing.T) {
	out, err := ViewObjectPage(&ViewObjectParams{
		CollectionDisplayName: "Sketches",
		CollectionLink:        "/show-collection?id=abc",
		Name:                  "cats/tabby <1>.png",
		SelfLink:              "/view-object?collection=abc&name=cats%2Ftabby+%3C1%3E.png",
		ContentLink:           "/collection-object?collection=abc&name=cats%2Ftabby+%3C1%3E.png",
		ContentType:           "image/png",
		IsImage:               true,
		NextLink:              "/view-object?collection=abc&name=dog.png",
		Tags:                  []string{"tabby", "<script>"},
		AddTagLink:            "/add-object-tag",
		CollectionID:          "abc",
		UserError:             "tag too long",
	})
	if err != nil {
		t.Fatalf("ViewObjectPage: %v", err)
	}
	for _, want := range []string{
		"cats/tabby &lt;1&gt;.png",
		`<img src="/collection-object?collection=abc&amp;name=cats%2Ftabby&#43;%3C1%3E.png"`,
		`href="/view-object?collection=abc&amp;name=dog.png" rel="next"`,
		`disabled" aria-disabled="true">&larr; Previous`,
		"<td>tabby</td>",
		"<td>&lt;script&gt;</td>",
		`action="/add-object-tag"`,
		`name="collection" value="abc"`,
		`name="name" value="cats/tabby &lt;1&gt;.png"`,
		"Error: tag too long",
	} {
		if !strings.Contains(string(out), want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestViewObjectPageNoTags(t *testing.T) {
	out, err := ViewObjectPage(&ViewObjectParams{Name: "cat.png"})
	if err != nil {
		t.Fatalf("ViewObjectPage: %v", err)
	}
	if !strings.Contains(string(out), "No tags.") {
		t.Errorf("output missing empty-tags message")
	}
	if strings.Contains(string(out), "alert-danger") {
		t.Errorf("output unexpectedly contains an error alert")
	}
}
