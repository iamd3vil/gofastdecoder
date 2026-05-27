// Package decimalind is an example decoder for a decimal field with individual
// exponent and mantissa operators (exponent copy, mantissa delta).
//
//go:generate go run github.com/iamd3vil/gofastdecoder/fastgen -in px.xml -out px_gen.go -pkg decimalind
package decimalind
