// Package multifile is an example where a template (Quote, in feed.xml)
// statically references a template defined in a different file (Header, in
// common.xml). fastgen merges both files (repeated -in) and resolves the
// cross-file reference by inlining Header into Quote.
//
//go:generate go run github.com/iamd3vil/gofastdecoder/fastgen -in common.xml -in feed.xml -out feed_gen.go -pkg multifile
package multifile
