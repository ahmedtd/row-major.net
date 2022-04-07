// Command bump-manifest uses the Gitlab API to submit an updated Kubernetes
// Kustomize strategic merge patch file that bumps a particular container image
// to a new tag.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"text/template"
)

var (
	mergeFile         = flag.String("merge-file", "", "")
	namespace         = flag.String("namespace", "", "")
	name              = flag.String("name", "", "")
	containerName     = flag.String("container-name", "", "")
	containerImage    = flag.String("container-image", "", "")
	containerImageTag = flag.String("container-image-tag", "", "")

	mode = flag.String("mode", "", "")
)

var mergeTemplate = template.Must(template.New("").Parse(`# This file is overwritten by CI.
#
# Refer to //infra-tools/bump-manifest to see how it is generated.
apiVersion: apps/v1
kind: Deployment
metadata:
  namespace: {{.Namespace}}
  name: {{.Name}}
spec:
  template:
    spec:
      containers:
      - name: {{.ContainerName}}
        image: {{.ContainerImage}}:{{.ContainerImageTag}}
`))

type MergeTemplateArgs struct {
	Namespace         string
	Name              string
	ContainerName     string
	ContainerImage    string
	ContainerImageTag string
}

var commitMessageTemplate = template.Must(template.New("").Parse(`Bump {{.ContainerImage}} to {{.Tag}} in {{.Namespace}}/{{.Name}}

NO_TEST=
NO_UPLOAD_IMAGES=
NO_UPDATE_IMAGES_IN_MANIFEST=
`))

type CommitMessageTemplateArgs struct {
	ContainerImage string
	Tag            string
	Namespace      string
	Name           string
}

type Command struct {
	MergeFile         string
	Namespace         string
	Name              string
	ContainerName     string
	ContainerImage    string
	ContainerImageTag string
}

type CreateCommitRequest struct {
	Branch        string               `json:"branch,omitempty"`
	CommitMessage string               `json:"commit_message,omitempty"`
	AuthorEmail   string               `json:"author_email,omitempty"`
	AuthorName    string               `json:"author_name,omitempty"`
	Actions       []CreateCommitAction `json:"actions,omitempty"`
}

type CreateCommitAction struct {
	Action          string `json:"action,omitempty"`
	FilePath        string `json:"file_path,omitempty"`
	PreviousPath    string `json:"previous_path,omitempty"`
	Content         string `json:"content,omitempty"`
	Encoding        string `json:"encoding,omitempty"`
	LastCommitID    string `json:"last_commit_id,omitempty"`
	ExecuteFilemode string `json:"execute_filemode,omitempty"`
}

func (c *Command) Do(ctx context.Context) error {
	ciAPIV4URL, ok := os.LookupEnv("CI_API_V4_URL")
	if !ok {
		return fmt.Errorf("CI_API_V4_URL environment variable unset")
	}

	ciProjectID, ok := os.LookupEnv("CI_PROJECT_ID")
	if !ok {
		return fmt.Errorf("CI_PROJECT_ID environment variable unset")
	}

	ciCommitBranch, ok := os.LookupEnv("CI_COMMIT_BRANCH")
	if !ok {
		return fmt.Errorf("CI_COMMIT_BRANCH environment variable unset")
	}

	gitlabAPIToken, ok := os.LookupEnv("GITLAB_API_TOKEN")
	if !ok {
		return fmt.Errorf("GITLAB_API_TOKEN environment variable unset")
	}

	mergeContentBuilder := &strings.Builder{}
	mergeTemplateArgs := MergeTemplateArgs{
		Namespace:         c.Namespace,
		Name:              c.Name,
		ContainerName:     c.ContainerName,
		ContainerImage:    c.ContainerImage,
		ContainerImageTag: c.ContainerImageTag,
	}
	if err := mergeTemplate.Execute(mergeContentBuilder, mergeTemplateArgs); err != nil {
		return fmt.Errorf("while templating merge file content: %w", err)
	}

	commitMessageBuilder := &strings.Builder{}
	commitMessageArgs := CommitMessageTemplateArgs{
		ContainerImage: c.ContainerImage,
		Tag:            c.ContainerImageTag,
		Namespace:      c.Namespace,
		Name:           c.Name,
	}
	if err := commitMessageTemplate.Execute(commitMessageBuilder, commitMessageArgs); err != nil {
		return fmt.Errorf("while templating commit message: %w", err)
	}

	bodyBytes, err := json.Marshal(CreateCommitRequest{
		Branch:        ciCommitBranch,
		CommitMessage: commitMessageBuilder.String(),
		Actions: []CreateCommitAction{
			{
				Action:   "update",
				FilePath: c.MergeFile,
				Content:  mergeContentBuilder.String(),
			},
		},
	})

	apiURL := ciAPIV4URL + "projects/" + ciProjectID + "/repository/commits"

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("while creating request: %w", err)
	}
	req.Header.Add("Private-Token", gitlabAPIToken)
	req.Header.Add("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("while executing request: %w", err)
	}
	defer resp.Body.Close()

	responseBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("while reading response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("response had error status %d, body: %s", resp.StatusCode, string(responseBytes))
	}

	return nil
}

func do() error {
	c := &Command{
		MergeFile:         *mergeFile,
		Namespace:         *namespace,
		Name:              *name,
		ContainerName:     *containerName,
		ContainerImage:    *containerImage,
		ContainerImageTag: *containerImageTag,
	}

	if err := c.Do(context.Background()); err != nil {
		return err
	}

	return nil
}

func main() {
	flag.Parse()

	if err := do(); err != nil {
		log.Printf("Error: %v", err)
		os.Exit(1)
	}
}
