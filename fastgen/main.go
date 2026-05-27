// Command fastgen generates a Go decoder from a FAST template XML file.
//
// Usage:
//
//	fastgen -in templates.xml -out msgs_gen.go -pkg mypkg
//
// It is designed to be driven by `go:generate`:
//
//	//go:generate fastgen -in templates.xml -out msgs_gen.go -pkg mypkg
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/iamd3vil/gofastdecoder/fastgen/gen"
	"github.com/iamd3vil/gofastdecoder/fastgen/parser"
)

func main() {
	in := flag.String("in", "", "input FAST template XML file (required)")
	out := flag.String("out", "", "output Go file (default: stdout)")
	pkg := flag.String("pkg", "", "package name for the generated file (required)")
	flag.Parse()

	if *in == "" || *pkg == "" {
		flag.Usage()
		os.Exit(2)
	}

	if err := run(*in, *out, *pkg); err != nil {
		fmt.Fprintln(os.Stderr, "fastgen:", err)
		os.Exit(1)
	}
}

func run(in, out, pkg string) error {
	schema, err := parser.ParseFile(in)
	if err != nil {
		return fmt.Errorf("parse %s: %w", in, err)
	}
	src, err := gen.Generate(schema, pkg)
	if err != nil {
		return fmt.Errorf("generate: %w", err)
	}
	if out == "" {
		_, err = os.Stdout.Write(src)
		return err
	}
	if err := os.WriteFile(out, src, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", out, err)
	}
	return nil
}
