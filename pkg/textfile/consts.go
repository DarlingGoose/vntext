package textfile

type EncodingName string

const (
	EncodingASCII      EncodingName = "ascii"
	EncodingUTF8       EncodingName = "utf-8"
	EncodingUTF8BOM    EncodingName = "utf-8-bom"
	EncodingUTF16LEBOM EncodingName = "utf-16le-bom"
	EncodingUTF16BEBOM EncodingName = "utf-16be-bom"
	EncodingShiftJIS   EncodingName = "shift-jis/cp932"
	EncodingEUCJP      EncodingName = "euc-jp"
)

type NewlineStyle string

const (
	NewlineLF   NewlineStyle = "lf"
	NewlineCRLF NewlineStyle = "crlf"
	NewlineCR   NewlineStyle = "cr"
)
