// Package templateref is an example where one template statically references
// another via <templateRef name="Common"/>, which fastgen inlines.
//
//go:generate go run github.com/iamd3vil/gofastdecoder/fastgen -in tr.xml -out tr_gen.go -pkg templateref
package templateref
