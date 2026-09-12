package buildinfo

// Эти значения заменяются во время сборки через -ldflags. Значения по умолчанию делают
// локальные development-сборки явными и воспроизводимыми.
var (
	Version   = "0.31.0"
	Commit    = "unknown"
	BuildTime = "unknown"
)

const (
	ProductName = "control-center"
	APIVersion  = "v1"
)
