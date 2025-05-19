package types

// Config represents the application configuration that can be loaded from a file.
type Config struct {
	Profiles   []string `json:"profiles" yaml:"profiles" toml:"profiles"`
	Regions    []string `json:"regions" yaml:"regions" toml:"regions"`
	Combine    bool     `json:"combine" yaml:"combine" toml:"combine"`
	ReportName string   `json:"report_name" yaml:"report_name" toml:"report_name"`
	ReportType []string `json:"report_type" yaml:"report_type" toml:"report_type"`
	Dir        string   `json:"dir" yaml:"dir" toml:"dir"`
	TimeRange  int      `json:"time_range" yaml:"time_range" toml:"time_range"`
	Tag        []string `json:"tag" yaml:"tag" toml:"tag"`
	Audit      bool     `json:"audit" yaml:"audit" toml:"audit"`
	Trend      bool     `json:"trend" yaml:"trend" toml:"trend"`
}
