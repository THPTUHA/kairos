package extcron

import "github.com/robfig/cron/v3"

// ExtParser is a parser extending robfig/cron v3 standard parser with
// several additional descriptors
type ExtParser struct {
	parser cron.Parser
}

// NewParser creates an ExtParser instance
func NewParser() cron.ScheduleParser {
	return ExtParser{cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)}
}

// TODO
// Parse parses a cron schedule specification. It accepts the cron spec with
// mandatory seconds parameter, descriptors and the custom descriptors
// "@at <date>", "@manually" and "@minutely".
func (p ExtParser) Parse(spec string) (cron.Schedule, error) {
	return nil, nil
}
