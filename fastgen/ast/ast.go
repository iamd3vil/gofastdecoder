// Package ast is the intermediate representation produced by the fastgen parser
// and consumed by the code emitter. It models a FAST template set after the
// 1.2 desugaring rules have been applied and all named-type (<define>/<type>)
// references have been resolved/inlined — so a consumer never sees unresolved
// references and can walk templates → instructions → fields directly.
package ast

// BaseType enumerates FAST field base types: the FAST 1.1 scalar/string/binary
// types plus the FAST 1.2 extension types.
type BaseType int

const (
	Int32 BaseType = iota
	UInt32
	Int64
	UInt64
	Decimal
	ASCIIString
	UnicodeString
	ByteVector
	// FAST 1.2 extensions.
	Enum
	Set
	Boolean
	Timestamp
	BitGroup
)

// Presence is a field's presence (§6.2 presenceAttr).
type Presence int

const (
	Mandatory Presence = iota
	Optional
)

// OpKind identifies a field operator (§6.3). NoOp means the field has no
// operator instruction (encoded directly / nullable when optional).
type OpKind int

const (
	NoOp OpKind = iota
	Constant
	Default
	Copy
	Increment
	Delta
	Tail
)

// Op is an operator instance attached to a field (or to a decimal's exponent or
// mantissa, or to a sequence length). Initial holds the raw value attribute as
// written in the template; it is converted to the field's type at emit time
// (§6.3.2). Dictionary is "", "global", "template", "type", or a user name;
// Key overrides the default key (the field name) when non-empty (§6.3.1).
type Op struct {
	Kind       OpKind
	HasInitial bool
	Initial    string
	Dictionary string
	Key        string
}

// Element is a named member of an enum or set (FAST 1.2). Value is the encoded
// integer assigned to the element.
type Element struct {
	Name  string
	Value int64
}

// Instruction is a template body entry: a Field, Sequence, or Group.
type Instruction interface{ instrNode() }

func (*Field) instrNode()    {}
func (*Sequence) instrNode() {}
func (*Group) instrNode()    {}

// Field is a scalar, string, binary, or FAST 1.2 extension field.
type Field struct {
	Name     string
	ID       uint32
	HasID    bool
	Type     BaseType
	Presence Presence

	// Op applies to most field types. For Decimal with individual operators,
	// Exponent and Mantissa are set instead and Op.Kind is NoOp.
	Op       Op
	Exponent *Op // decimal exponent operator, if specified individually
	Mantissa *Op // decimal mantissa operator, if specified individually

	// Elements lists the members of an Enum or Set field.
	Elements []Element

	// Unit and Epoch describe a Timestamp field ("day"/"second"/"millisecond"/
	// "microsecond"/"nanosecond"; epoch "" defaults to UNIX, "today" for time).
	Unit  string
	Epoch string

	// BitFields are the packed sub-fields of a BitGroup, in wire order
	// (left-to-right bit allocation).
	BitFields []*Field

	// BitWidth is the fixed bit width of a field packed inside a BitGroup. It is
	// set for the FAST 1.2 int2..int7 / uInt1..uInt7 sub-types; for enum, set,
	// and boolean sub-fields it is 0 and the width is derived from the type
	// (enum: ceil(log2(n)); set: n; boolean: 1, or 2 if optional).
	BitWidth int
}

// Sequence is a repeating group: a length field (uInt32, §6.2.5) followed by a
// body repeated that many times, each repetition carrying its own presence map.
type Sequence struct {
	Name         string
	ID           uint32
	HasID        bool
	Presence     Presence
	Dictionary   string
	Length       *Field // the length field, with its own operator
	Instructions []Instruction
}

// Group is a static nested block; optional groups consume one presence-map bit.
type Group struct {
	Name         string
	ID           uint32
	HasID        bool
	Presence     Presence
	Dictionary   string
	Instructions []Instruction
}

// Template is a top-level template definition.
type Template struct {
	Name         string
	ID           uint32
	HasID        bool
	Namespace    string
	Dictionary   string
	TypeRef      string // application type (for the "type" dictionary scope)
	Instructions []Instruction
}

// Schema is a parsed template set (one <templates> document).
type Schema struct {
	Namespace  string
	TemplateNs string
	Dictionary string
	Templates  []*Template
}
