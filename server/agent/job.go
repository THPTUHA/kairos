package agent

// Job describes a scheduled Job.
type Job struct {
	// Job id. Must be unique, it's a copy of name.
	ID string `json:"id"`
	// Job name. Must be unique, acts as the id.
	Name string `json:"name"`
}
