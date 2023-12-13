package deliverer

type PresenceStats struct {
	NumClients int
	NumUsers   int
}

type PresenceManager interface {
	Presence(ch string) (map[string]*ClientInfo, error)
	PresenceStats(ch string) (PresenceStats, error)
	AddPresence(ch string, clientID string, info *ClientInfo) error
	RemovePresence(ch string, clientID string) error
}
