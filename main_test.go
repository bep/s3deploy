// Copyright © 2022 Bjørn Erik Pedersen <bjorn.erik.pedersen@gmail.com>.
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/rogpeppe/go-internal/testscript"
)

const s3IntegrationTestHttpRoot = "http://s3deployintegrationtest.s3-website.eu-north-1.amazonaws.com"

func TestIntegration(t *testing.T) {
	if os.Getenv("S3DEPLOY_TEST_KEY") == "" {
		t.Skip("S3DEPLOY_TEST_KEY not set")

	}
	testscript.Run(t, testscript.Params{
		Dir: "testscripts",
		//UpdateScripts: true,
		Setup: func(env *testscript.Env) error {
			return setup(env)
		},
	})
}

// Tests in development can be put in "testscripts/unfinished".
func TestUnfinished(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skip unfinished tests on CI")
	}

	testscript.Run(t, testscript.Params{
		Dir: "testscripts/unfinished",
		//TestWork: true,
		//UpdateScripts: true,
		Setup: func(env *testscript.Env) error {
			return setup(env)
		},
	})
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

			// head executes HTTP HEAD on the given URL and prints the response status code and
			// headers to stdout.
			"head": func() int {
				url := s3IntegrationTestHttpRoot + os.Args[1]
				fmt.Println("head", url)
				resp, err := http.DefaultClient.Head(url)
				if err != nil {
					log.Fatal(err)
				}
				path := strings.ReplaceAll(os.Args[1], os.Getenv("S3DEPLOY_TEST_ID"), "S3DEPLOY_TEST_ID")
				fmt.Printf("Head: %s;Status: %d;", path, resp.StatusCode)
				// Print headers
				var headers []string
				for k, v := range resp.Header {
					headers = append(headers, fmt.Sprintf("%s: %s", k, v[0]))
				}
				sort.Strings(headers)
				fmt.Printf("Headers: %s", strings.Join(headers, ";"))

				return 0
			},

			// append appends to a file with a leaading newline.
			"append": func() int {
				if len(os.Args) < 3 {

					fmt.Fprintln(os.Stderr, "usage: append FILE TEXT")
					return 1
				}

				filename := os.Args[1]
				words := os.Args[2:]
				for i, word := range words {
					words[i] = strings.Trim(word, "\"")
				}
				text := strings.Join(words, " ")

				_, err := os.Stat(filename)
				if err != nil {
					if os.IsNotExist(err) {
						fmt.Fprintln(os.Stderr, "file does not exist:", filename)
						return 1
					}
					fmt.Fprintln(os.Stderr, err)
					return 1
				}

				f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY, 0o644)
				if err != nil {
					fmt.Fprintln(os.Stderr, "failed to open file:", filename)
					return 1
				}
				defer f.Close()

				_, err = f.WriteString("\n" + text)
				if err != nil {
					fmt.Fprintln(os.Stderr, "failed to write to file:", filename)
					return 1
				}

				return 0
			},

			// log prints to stderr.
			"log": func() int {
				log.Println(os.Args[1])
				return 0
			},

			"sleep": func() int {
				i, err := strconv.Atoi(os.Args[1])
				if err != nil {
					i = 1
				}
				time.Sleep(time.Duration(i) * time.Second)
				return 0
			},
			"unset": func() int {
				os.Unsetenv(os.Args[1])
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
