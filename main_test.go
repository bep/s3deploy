// Copyright 2022 The Hugoreleaser Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bep/helpers/envhelpers"
	"github.com/bep/helpers/filehelpers"
	"github.com/rogpeppe/go-internal/testscript"
)

// Note: If tests are running slow for you, make sure you have GOMODCACHE set.
func TestCommands(t *testing.T) {
	setup := testSetupFunc()
	testscript.Run(t, testscript.Params{
		Dir: "testscripts/commands",
		Setup: func(env *testscript.Env) error {
			return setup(env)
		},
	})
}

func TestMisc(t *testing.T) {
	setup := testSetupFunc()
	testscript.Run(t, testscript.Params{
		Dir: "testscripts/misc",
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

	setup := testSetupFunc()

	testscript.Run(t, testscript.Params{
		Dir: "testscripts/unfinished",
		//TestWork: true,
		//UpdateScripts: true,
		Setup: func(env *testscript.Env) error {
			return setup(env)
		},
	})
}

func testSetupFunc() func(env *testscript.Env) error {
	sourceDir, _ := os.Getwd()
	return func(env *testscript.Env) error {
		var keyVals []string

		// SOURCE is where the hugoreleaser source code lives.
		// We do this so we can
		// 1. Copy the example/test plugins into the WORK dir where the test script is running.
		// 2. Append a replace directive to the plugins' go.mod to get the up-to-date version of the plugin API.
		//
		// This is a hybrid setup neeed to get a quick development cycle going.
		// In production, the plugin Go modules would be addressed on their full form, e.g. "github.com/gohugoio/hugoreleaser/internal/plugins/archives/tar@v1.0.0".
		keyVals = append(keyVals, "SOURCE", sourceDir)
		keyVals = append(keyVals, "GOCACHE", filepath.Join(env.WorkDir, "gocache"))
		var gomodCache string
		if c := os.Getenv("GOMODCACHE"); c != "" {
			// Testscripts will set the GOMODCACHE to an empty dir,
			// and this slows down some tests considerably.
			// Use the OS env var if it is set.
			gomodCache = c
		} else {
			gomodCache = filepath.Join(env.WorkDir, "gomodcache")
		}
		keyVals = append(keyVals, "GOMODCACHE", gomodCache)

		envhelpers.SetEnvVars(&env.Vars, keyVals...)

		return nil
	}
}

func TestMain(m *testing.M) {
	os.Exit(
		testscript.RunMain(m, map[string]func() int{
			// The main program.
			"hugoreleaser": func() int {
				if err := parseAndRun(os.Args[1:]); err != nil {
					fmt.Fprintln(os.Stderr, err)
					return 1
				}
				return 0
			},

			// dostounix converts \r\n to \n.
			"dostounix": func() int {
				filename := os.Args[1]
				b, err := os.ReadFile(filename)
				if err != nil {
					fatalf("%v", err)
				}
				b = bytes.Replace(b, []byte("\r\n"), []byte{'\n'}, -1)
				if err := os.WriteFile(filename, b, 0666); err != nil {
					fatalf("%v", err)
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

			// ls lists a directory to stdout.
			"ls": func() int {
				dirname := os.Args[1]
				dir, err := os.Open(dirname)
				if err != nil {
					fatalf("%v", err)
				}
				fis, err := dir.Readdir(-1)
				if err != nil {
					fatalf("%v", err)
				}
				for _, fi := range fis {
					fmt.Printf("%s %04o %s\n", fi.Mode(), fi.Mode().Perm(), fi.Name())
				}
				return 0
			},

			// printarchive prints the contents of an archive to stdout.
			"printarchive": func() int {
				archiveFilename := os.Args[1]

				if !strings.HasSuffix(archiveFilename, ".tar.gz") {
					fatalf("only .tar.gz supported for now, got: %q", archiveFilename)
				}

				f, err := os.Open(archiveFilename)
				if err != nil {
					fatalf("%v", err)
				}
				defer f.Close()

				gr, err := gzip.NewReader(f)
				if err != nil {
					fatalf("%v", err)
				}
				defer gr.Close()
				tr := tar.NewReader(gr)

				for {
					hdr, err := tr.Next()
					if err == io.EOF {
						break
					}
					if err != nil {
						fatalf("%v", err)
					}
					mode := fs.FileMode(hdr.Mode)
					fmt.Printf("%s %04o %s\n", mode, mode.Perm(), hdr.Name)
				}

				return 0

			},

			// cpdir copies a file.
			"cpfile": func() int {
				if len(os.Args) != 3 {
					fmt.Fprintln(os.Stderr, "usage: cpdir SRC DST")
					return 1
				}

				fromFile := os.Args[1]
				toFile := os.Args[2]

				if !filepath.IsAbs(fromFile) {
					fromFile = filepath.Join(os.Getenv("SOURCE"), fromFile)
				}

				if err := os.MkdirAll(filepath.Dir(toFile), 0755); err != nil {
					fmt.Fprintln(os.Stderr, err)
					return 1
				}

				err := filehelpers.CopyFile(fromFile, toFile)
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					return 1
				}
				return 0
			},

			// cpdir copies a directory recursively.
			"cpdir": func() int {
				if len(os.Args) != 3 {
					fmt.Fprintln(os.Stderr, "usage: cpdir SRC DST")
					return 1
				}

				fromDir := os.Args[1]
				toDir := os.Args[2]

				if !filepath.IsAbs(fromDir) {
					fromDir = filepath.Join(os.Getenv("SOURCE"), fromDir)
				}

				err := filehelpers.CopyDir(fromDir, toDir, nil)
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					return 1
				}
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

			// Helpers.
			"checkfile": func() int {
				// The built-in exists does not check for zero size files.
				args := os.Args[1:]
				var readonly, exec bool
			loop:
				for len(args) > 0 {
					switch args[0] {
					case "-readonly":
						readonly = true
						args = args[1:]
					case "-exec":
						exec = true
						args = args[1:]
					default:
						break loop
					}
				}
				if len(args) == 0 {
					fatalf("usage: checkfile [-readonly] [-exec] file...")
				}

				for _, filename := range args {

					fi, err := os.Stat(filename)
					if err != nil {
						fmt.Fprintf(os.Stderr, "stat %s: %v\n", filename, err)
						return -1
					}
					if fi.Size() == 0 {
						fmt.Fprintf(os.Stderr, "%s is empty\n", filename)
						return -1
					}
					if readonly && fi.Mode()&0o222 != 0 {
						fmt.Fprintf(os.Stderr, "%s is writable\n", filename)
						return -1
					}
					if exec && runtime.GOOS != "windows" && fi.Mode()&0o111 == 0 {
						fmt.Fprintf(os.Stderr, "%s is not executable\n", filename)
						return -1
					}
				}

				return 0
			},
			"checkfilecount": func() int {
				if len(os.Args) != 3 {
					fatalf("usage: checkfilecount count dir")
				}

				count, err := strconv.Atoi(os.Args[1])
				if err != nil {
					fatalf("invalid count: %v", err)
				}
				if count < 0 {
					fatalf("count must be non-negative")
				}
				dir := os.Args[2]

				found := 0

				filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if d.IsDir() {
						return nil
					}
					found++
					return nil
				})

				if found != count {
					fmt.Fprintf(os.Stderr, "found %d files, want %d\n", found, count)
					return -1
				}

				return 0
			},
			"gobinary": func() int {
				if runtime.GOOS == "windows" {
					return 0
				}
				if len(os.Args) < 3 {
					fatalf("usage: gobinary binary args...")
				}

				filename := os.Args[1]
				pattern := os.Args[2]
				if !strings.HasPrefix(pattern, "(") {
					// Multiline matching.
					pattern = "(?s)" + pattern
				}
				re := regexp.MustCompile(pattern)

				cmd := exec.Command("go", "version", "-m", filename)
				cmd.Stderr = os.Stderr

				b, err := cmd.Output()
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					return -1
				}

				output := string(b)

				if !re.MatchString(output) {
					fmt.Fprintf(os.Stderr, "expected %q to match %q\n", output, re)
					return -1
				}

				return 0
			},
		}),
	)
}

func fatalf(format string, a ...any) {
	panic(fmt.Sprintf(format, a...))
}
