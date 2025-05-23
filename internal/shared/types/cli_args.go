package types

// CLIArgs represents the command-line arguments.
type CLIArgs struct {
	ConfigFile string
	Profiles   []string
	Regions    []string
	All        bool
	Combine    bool
	ReportName string
	ReportType []string
	Dir        string
	TimeRange  *int
	Tag        []string
	Trend      bool
	Audit      bool
}
