// Package ext12 is an example decoder exercising FAST 1.2 extension field
// types (boolean, timestamp, enum) through the fastgen pipeline.
//
//go:generate go run github.com/iamd3vil/gofastdecoder/fastgen -in ext.xml -out ext_gen.go -pkg ext12
package ext12
