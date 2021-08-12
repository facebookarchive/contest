// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"text/template"

	flag "github.com/spf13/pflag"
)

var (
	flagTemplate = flag.StringP("template", "t", "contest.go.template", "Template file name")
	flagOutfile  = flag.StringP("outfile", "o", "", "Output file path")
	flagFromFile = flag.StringP("from", "f", "core-plugins.yml", "File name to get the plugin list from, in YAML format")
)

func usage() {
	fmt.Fprintf(flag.CommandLine.Output(),
		"%s [--template tpl] [--outfile out] [--from pluginlist.yml]\n",
		os.Args[0],
	)
	flag.PrintDefaults()
}

func main() {
	flag.Usage = usage
	flag.Parse()

	data, err := ioutil.ReadFile(*flagTemplate)
	if err != nil {
		log.Fatalf("Failed to read template file '%s': %v", *flagTemplate, err)
	}
	t, err := template.New("contest").Parse(string(data))
	if err != nil {
		log.Fatalf("Template parsing failed: %v", err)
	}
	cfg, err := readConfig(*flagFromFile)
	if err != nil {
		log.Fatalf("Error parsing configuration file '%s': %v", *flagFromFile, err)
	}
	outfile := *flagOutfile
	if outfile == "" {
		tmpdir, err := ioutil.TempDir("", "contest")
		if err != nil {
			log.Fatalf("Cannot create temporary directory: %v", err)
		}
		outfile = path.Join(tmpdir, "contest.go")
	}

	log.Printf("Generating output file '%s' with the following plugins):\n%s", outfile, cfg)
	outFD, err := os.OpenFile(outfile, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to create output file '%s': %v", outfile, err)
	}
	defer func() {
		if err := outFD.Close(); err != nil {
			log.Printf("Error while closing file descriptor for '%s': %v", outfile, err)
		}
	}()
	if err := t.Execute(outFD, cfg); err != nil {
		log.Fatalf("Template execution failed: %v", err)
	}
	log.Printf("Generated file '%s'. You can build it by running 'go build' in the output directory.", outfile)
	fmt.Println(path.Dir(outfile))
}
