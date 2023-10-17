package agent

import (
	"net"

	version "github.com/hashicorp/go-version"
	"github.com/hashicorp/serf/serf"
)

type ServerParts struct {
	Name         string
	ID           string
	Region       string
	Datacenter   string
	Port         int
	Bootstrap    bool
	Expect       int
	RaftVersion  int
	BuildVersion *version.Version
	Addr         net.Addr
	RPCAddr      net.Addr
	Status       serf.MemberStatus
}
