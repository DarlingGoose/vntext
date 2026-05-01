package textfile

type FileStyle struct {
	Encoding EncodingName
	Newline  NewlineStyle

	// HadFinalNewline lets you preserve whether the file ended with \n.
	HadFinalNewline bool
}

type TextFile struct {
	Path  string
	Text  string // Always decoded as UTF-8 internally.
	Style FileStyle
	Raw   []byte
}
