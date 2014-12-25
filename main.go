package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func walk(basePath string) {
	fmt.Println("walking", basePath)

	filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// skip hidden directories like .git
			if strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
		} else {
			rel, err := filepath.Rel(basePath, path)
			if err != nil {
				return err
			}
			// can use size and time (or possibly md5) to determine what needs to upload
			fmt.Println(rel, info.Size(), info.ModTime().Unix())
		}
		return nil
	})
}

func main() {
	var source string
	var help bool
	flag.StringVar(&source, "source", ".", "path of files to upload")
	flag.BoolVar(&help, "h", false, "help")
	flag.Parse()

	if help {
		flag.Usage()
		return
	}

	walk(source)
}
