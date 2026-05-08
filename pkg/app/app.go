package app

const DefaultProgramName = "vntext"

var ProgramName = DefaultProgramName

func Name() string {
	if ProgramName == "" {
		return DefaultProgramName
	}
	return ProgramName
}
