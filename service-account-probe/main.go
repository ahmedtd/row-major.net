package main

import (
	"context"
	"flag"
	"io/ioutil"
	"net/http"

	"github.com/golang/glog"
	"golang.org/x/oauth2/google"
)

func main() {
	flag.Parse()

	glog.CopyStandardLogTo("INFO")

	ctx := context.Background()

	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		glog.Fatalf("Couldn't find application default credentials: %v", err)
	}

	glog.Infof("Current credentials: %v", string(creds.JSON))

	tok, err := creds.TokenSource.Token()
	if err != nil {
		glog.Fatalf("Error creating token: %v", err)
	}

	resp, err := http.Get("https://oauth2.googleapis.com/tokeninfo?access_token=" + tok.AccessToken)
	if err != nil {
		glog.Fatalf("Error calling tokeninfo endpoint: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		glog.Fatalf("Error reading tokeninfo response: %v", err)
	}

	glog.Infof("Token Info: %s", string(body))

	glog.Flush()
}
