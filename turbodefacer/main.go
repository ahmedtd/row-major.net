package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/textproto"
	"strings"
	"time"

	"github.com/jlaffaye/ftp"
)

var (
	connStr   = flag.String("connection-string", "", "Where to connect (<host>:<port>).")
	credsFile = flag.String("creds-file", "", "File with credentials.")
	user      = flag.String("user", "", "Username to use.")
	pass      = flag.String("password", "", "Password to use.")
)

func main() {
	flag.Parse()

	c, err := ftp.Dial(*connStr, ftp.DialWithTimeout(5*time.Second))
	if err != nil {
		log.Fatalf("While dialing %q: %v", *connStr, err)
	}

	user := *user
	pass := *pass

	if *credsFile != "" {
		user, pass, err = readCreds(*credsFile)
	}

	err = c.Login(user, pass)
	if err != nil {
		log.Fatalf("While logging in as %q: %v", err)
	}

	err = ensureBackup(c, "index.html")
	if err != nil {
		log.Fatalf("While creating backup file: %v", err)
	}

	backupContent, err := retrieveContent(c, "index.html.bak")
	if err != nil {
		log.Fatalf("While retrieving backup: %v", err)
	}
	fmt.Print(string(backupContent))

	if err := c.Quit(); err != nil {
		log.Fatalf("While closing connection: %v", err)
	}
}

func ensureBackup(c *ftp.ServerConn, path string) error {
	primaryContent, err := retrieveContent(c, path)
	if err != nil {
		return fmt.Errorf("while retrieving primary file: %w", err)
	}

	_, err = retrieveContent(c, path+".bak")
	if err != nil {
		tpe := &textproto.Error{}
		if !errors.As(err, &tpe) || tpe.Code != 550 {
			return fmt.Errorf("while retrieving backup file: %w", err)
		}

		// Code was 550; need to create the backup file.
		encodedPrimaryContent := []byte(base64.StdEncoding.EncodeToString(primaryContent))
		err := storeContent(c, path+".bak", encodedPrimaryContent)
		if err != nil {
			return fmt.Errorf("while creating backup file: %w", err)
		}
		return nil
	}

	return nil
}

func retrieveContent(c *ftp.ServerConn, path string) ([]byte, error) {
	resp, err := c.Retr(path)
	if err != nil {
		return nil, fmt.Errorf("while retrieving: %w", err)
	}
	defer resp.Close()

	b, err := ioutil.ReadAll(resp)
	if err != nil {
		return nil, fmt.Errorf("while reading: %w", err)
	}

	return b, nil
}

func storeContent(c *ftp.ServerConn, path string, content []byte) error {
	r := bytes.NewReader(content)

	err := c.Stor(path, r)
	if err != nil {
		return fmt.Errorf("while writing: %w", err)
	}

	return nil
}

func readCreds(credsFile string) (string, string, error) {
	b, err := ioutil.ReadFile(credsFile)
	if err != nil {
		return "", "", fmt.Errorf("while reading file: %w", err)
	}

	s := string(b)

	lines := strings.Split(s, "\n")

	if len(lines) < 2 {
		return "", "", fmt.Errorf("not enough lines in file")
	}

	user := lines[0]
	pass := lines[1]

	if user == "" {
		return "", "", fmt.Errorf("user not specified")
	}
	if pass == "" {
		return "", "", fmt.Errorf("password not specified")
	}

	return user, pass, nil
}
