// Command fastgen generates a Go decoder from a FAST template XML file.
//
// Usage:
//
//	fastgen -in templates.xml -out msgs_gen.go -pkg mypkg
//
// -in may be repeated to merge several template files into one decoder, so a
// template defined in one file can be referenced from another (§6.4):
//
//	fastgen -in common.xml -in feed.xml -out msgs_gen.go -pkg mypkg
//
// It is designed to be driven by `go:generate`:
//
//	//go:generate fastgen -in templates.xml -out msgs_gen.go -pkg mypkg
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/iamd3vil/gofastdecoder/fastgen/gen"
	"github.com/iamd3vil/gofastdecoder/fastgen/parser"
)

// multiFlag collects a repeatable string flag (e.g. -in a -in b).
type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func main() {
	var ins multiFlag
	flag.Var(&ins, "in", "input FAST template XML file (required; repeatable to merge files)")
	out := flag.String("out", "", "output Go file (default: stdout)")
	pkg := flag.String("pkg", "", "package name for the generated file (required)")
	flag.Parse()

	if len(ins) == 0 || *pkg == "" {
		flag.Usage()
		os.Exit(2)
	}

	if err := run(ins, *out, *pkg); err != nil {
		fmt.Fprintln(os.Stderr, "fastgen:", err)
		os.Exit(1)
	}
}

func run(ins []string, out, pkg string) error {
	schema, err := parser.ParseFiles(ins...)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
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
