// Copyright © 2022 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/oklog/ulid/v2"

	"github.com/rogpeppe/go-internal/testscript"
)

const s3IntegrationTestHttpRoot = "http://s3deployintegrationtest.s3-website.eu-north-1.amazonaws.com"

func TestIntegration(t *testing.T) {
	if os.Getenv("S3DEPLOY_TEST_KEY") == "" {
		t.Skip("S3DEPLOY_TEST_KEY not set")

	}
	p := commonTestScriptsParam
	p.Dir = "testscripts"
	testscript.Run(t, p)
}

// Tests in development can be put in "testscripts/unfinished".
func TestUnfinished(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skip unfinished tests on CI")
	}
	p := commonTestScriptsParam
	p.Dir = "testscripts/unfinished"
	testscript.Run(t, p)

}

func TestMain(m *testing.M) {
	os.Exit(
		testscript.RunMain(m, map[string]func() int{
			// The main program.
			"s3deploy": func() int {
				if err := parseAndRun(os.Args[1:]); err != nil {
					fmt.Fprintln(os.Stderr, err)
					return 1
				}
				return 0
			},
		}),
	)
}

func setup(env *testscript.Env) error {
	env.Setenv("S3DEPLOY_TEST_KEY", os.Getenv("S3DEPLOY_TEST_KEY"))
	env.Setenv("S3DEPLOY_TEST_SECRET", os.Getenv("S3DEPLOY_TEST_SECRET"))
	env.Setenv("S3DEPLOY_TEST_BUCKET", "s3deployintegrationtest")
	env.Setenv("S3DEPLOY_TEST_REGION", "eu-north-1")
	env.Setenv("S3DEPLOY_TEST_URL", s3IntegrationTestHttpRoot)
	env.Setenv("S3DEPLOY_TEST_ID", strings.ToLower(ulid.Make().String()))
	return nil
}

var commonTestScriptsParam = testscript.Params{
	Setup: func(env *testscript.Env) error {
		return setup(env)
	},
	Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){

		// head executes HTTP HEAD on the given URL and prints the response status code and
		// headers to stdout.
		"head": func(ts *testscript.TestScript, neg bool, args []string) {
			url := s3IntegrationTestHttpRoot + args[0]
			fmt.Fprintln(ts.Stdout(), "head", url)
			resp, err := http.DefaultClient.Head(url)
			if err != nil {
				ts.Fatalf("failed to HEAD %s: %v", url, err)
			}
			path := strings.ReplaceAll(args[0], ts.Getenv("S3DEPLOY_TEST_ID"), "S3DEPLOY_TEST_ID")
			fmt.Fprintf(ts.Stdout(), "Head: %s;Status: %d;", path, resp.StatusCode)
			// Print headers
			var headers []string
			for k, v := range resp.Header {
				headers = append(headers, fmt.Sprintf("%s: %s", k, v[0]))
			}
			sort.Strings(headers)
			fmt.Fprintf(ts.Stdout(), "Headers: %s", strings.Join(headers, ";"))

		},

		// append appends to a file with a leaading newline.
		"append": func(ts *testscript.TestScript, neg bool, args []string) {
			if len(args) < 2 {
				ts.Fatalf("usage: append FILE TEXT")
			}

			filename := ts.MkAbs(args[0])
			words := args[1:]
			for i, word := range words {
				words[i] = strings.Trim(word, "\"")
			}
			text := strings.Join(words, " ")

			_, err := os.Stat(filename)
			if err != nil {
				if os.IsNotExist(err) {
					ts.Fatalf("file does not exist: %s", filename)
				}
				ts.Fatalf("failed to stat file: %v", err)
			}

			f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0o644)
			if err != nil {
				ts.Fatalf("failed to open file: %v", err)
			}
			defer f.Close()

			_, err = f.WriteString("\n" + text)
			if err != nil {
				ts.Fatalf("failed to write to file: %v", err)
			}
		},
		// replace replaces a string in a file.
		"replace": func(ts *testscript.TestScript, neg bool, args []string) {
			if len(args) < 3 {
				ts.Fatalf("usage: replace FILE OLD NEW")
			}
			filename := ts.MkAbs(args[0])
			oldContent, err := os.ReadFile(filename)
			if err != nil {
				ts.Fatalf("failed to read file %v", err)
			}
			newContent := bytes.Replace(oldContent, []byte(args[1]), []byte(args[2]), -1)
			err = os.WriteFile(filename, newContent, 0o644)
			if err != nil {
				ts.Fatalf("failed to write file: %v", err)
			}
		},
	},
}
