package agent

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-sockaddr/template"
	flag "github.com/spf13/pflag"
)

type AgentConfig struct {
	// NodeName is the name we register as. Defaults to hostname.
	NodeName string `mapstructure:"node-name"`

	// BindAddr is the address on which all of kairos's services will
	// be bound. If not specified, this defaults to the first private ip address.
	BindAddr string `mapstructure:"bind-addr"`

	// Profile is used to select a timing profile for Serf. The supported choices
	// are "wan", "lan", and "local". The default is "lan"
	Profile string

	// HTTPAddr is the address on the UI web server will
	// be bound. If not specified, this defaults to all interfaces.
	HTTPAddr string `mapstructure:"http-addr"`

	// AdvertiseAddr is the address that the Serf and gRPC layer will advertise to
	// other members of the cluster. Can be used for basic NAT traversal
	// where both the internal ip:port and external ip:port are known.
	AdvertiseAddr string `mapstructure:"advertise-addr"`

	// DevMode is used for development purposes only and limits the
	// use of persistence or state.
	DevMode bool
	// ReconcileInterval controls how often we reconcile the strongly
	// consistent store with the Serf info. This is used to handle nodes
	// that are force removed, as well as intermittent unavailability during
	// leader election.
	ReconcileInterval time.Duration

	Server bool
	// LogLevel is the log verbosity level used.
	// It can be (debug|info|warn|error|fatal|panic).
	LogLevel string `mapstructure:"log-level"`

	// Datacenter is the datacenter this Agent server belongs to.
	Datacenter string

	// Region is the region this Agent server belongs to.
	Region string

	// Tags are used to attach key/value metadata to a node.
	Tags map[string]string `mapstructure:"tags"`

	// EncryptKey is the secret key to use for encrypting communication
	// traffic for Serf. The secret key must be exactly 32-bytes, base64
	// encoded. The easiest way to do this on Unix machines is this command:
	// "head -c32 /dev/urandom | base64" or use "dkron keygen". If this is
	// not specified, the traffic will not be encrypted.
	EncryptKey string `mapstructure:"encrypt"`

	// Bootstrap mode is used to bring up the first Agent server.  It is
	// required so that it can elect a leader without any other nodes
	// being present
	Bootstrap bool

	// BootstrapExpect tries to automatically bootstrap the Agent cluster,
	// by withholding peers until enough servers join.
	BootstrapExpect int `mapstructure:"bootstrap-expect"`

	// SerfReconnectTimeout is the amount of time to attempt to reconnect to a failed node before giving up and considering it completely gone
	SerfReconnectTimeout string `mapstructure:"serf-reconnect-timeout"`

	// RetryJoinLAN is a list of addresses to attempt to join when the
	// agent starts. Serf will continue to retry the join until it
	// succeeds or RetryMaxAttempts is reached.
	RetryJoinLAN []string `mapstructure:"retry-join"`

	// RetryMaxAttemptsLAN is used to limit the maximum attempts made
	// by RetryJoin to reach other nodes. If this is 0, then no limit
	// is imposed, and Serf will continue to try forever. Defaults to 0.
	RetryJoinMaxAttemptsLAN int `mapstructure:"retry-max"`

	// RetryIntervalLAN is the string retry interval. This interval
	// controls how often we retry the join for RetryJoin. This defaults
	// to 30 seconds.
	RetryJoinIntervalLAN time.Duration `mapstructure:"retry-interval"`

	// StartJoin is a list of addresses to attempt to join when the
	// agent starts. If Serf is unable to communicate with any of these
	// addresses, then the agent will error and exit.
	StartJoin []string `mapstructure:"join"`

	// AdvertiseRPCPort is the gRPC port advertised to clients. This should be reachable
	// by the other servers and clients.
	AdvertiseRPCPort int `mapstructure:"advertise-rpc-port"`

	// RPCPort is the gRPC port used by Agent. This should be reachable
	// by the other servers and clients.
	RPCPort int `mapstructure:"rpc-port"`

	// RaftMultiplier An integer multiplier used by Dkron servers to scale key
	// Raft timing parameters.
	RaftMultiplier int `mapstructure:"raft-multiplier"`

	// DataDir is the directory to store our state in
	DataDir string `mapstructure:"data-dir"`
}

var ErrResolvingHost = errors.New("error resolving hostname")

const (
	DefaultBindPort int = 8946
	DefaultRPCPort  int = 6868
)

func DefaultConfig() *AgentConfig {
	// TODO
	hostname, err := os.Hostname()
	if err != nil {
		log.Panic(err)
	}

	tags := map[string]string{}

	return &AgentConfig{
		NodeName:             hostname,
		BindAddr:             fmt.Sprintf("{{ GetPrivateIP }}:%d", DefaultBindPort),
		HTTPAddr:             ":8080",
		Profile:              "lan",
		LogLevel:             "info",
		RPCPort:              DefaultRPCPort,
		Tags:                 tags,
		DataDir:              "dkron.data",
		Datacenter:           "dc1",
		Region:               "global",
		ReconcileInterval:    60 * time.Second,
		RaftMultiplier:       1,
		SerfReconnectTimeout: "24h",
	}
}

func (c *AgentConfig) normalizeAddrs() error {
	if c.BindAddr != "" {
		ipStr, err := ParseSingleIPTemplate(c.BindAddr)
		if err != nil {
			return fmt.Errorf("bind address resolution failed: %v", err)
		}
		c.BindAddr = ipStr
	}

	if c.HTTPAddr != "" {
		ipStr, err := ParseSingleIPTemplate(c.HTTPAddr)
		if err != nil {
			return fmt.Errorf("HTTP address resolution failed: %v", err)
		}
		c.HTTPAddr = ipStr
	}
	addr, err := normalizeAdvertise(c.AdvertiseAddr, c.BindAddr, DefaultBindPort, c.DevMode)
	if err != nil {
		return fmt.Errorf("failed to parse advertise address (%v, %v, %v, %v): %w", c.AdvertiseAddr, c.BindAddr, DefaultBindPort, c.DevMode, err)
	}
	c.AdvertiseAddr = addr

	return nil
}

// isMissingPort returns true if an error is a "missing port" error from
// net.SplitHostPort.
func isMissingPort(err error) bool {
	// matches error const in net/ipsock.go
	const missingPort = "missing port in address"
	return err != nil && strings.Contains(err.Error(), missingPort)
}

// isTooManyColons returns true if an error is a "too many colons" error from
// net.SplitHostPort.
func isTooManyColons(err error) bool {
	// matches error const in net/ipsock.go
	const tooManyColons = "too many colons in address"
	return err != nil && strings.Contains(err.Error(), tooManyColons)
}

func normalizeAdvertise(addr string, bind string, defport int, dev bool) (string, error) {
	addr, err := ParseSingleIPTemplate(addr)
	if err != nil {
		return "", fmt.Errorf("Error parsing advertise address template: %v", err)
	}

	if addr != "" {
		// Default to using manually configured address
		_, _, err = net.SplitHostPort(addr)
		if err != nil {
			if !isMissingPort(err) && !isTooManyColons(err) {
				return "", fmt.Errorf("Error parsing advertise address %q: %v", addr, err)
			}

			// missing port, append the default
			return net.JoinHostPort(addr, strconv.Itoa(defport)), nil
		}

		return addr, nil
	}

	ips, err := net.LookupIP(bind)
	if err != nil {
		return "", ErrResolvingHost //fmt.Errorf("Error resolving bind address %q: %v", bind, err)
	}

	for _, ip := range ips {
		if ip.IsLinkLocalUnicast() || ip.IsGlobalUnicast() {
			return net.JoinHostPort(ip.String(), strconv.Itoa(defport)), nil
		}
		if ip.IsLoopback() {
			if dev {
				// loopback is fine for dev mode
				return net.JoinHostPort(ip.String(), strconv.Itoa(defport)), nil
			}
			return "", fmt.Errorf("defaulting advertise to localhost is unsafe, please set advertise manually")
		}
	}

	// Bind is not localhost but not a valid advertise IP, use first private IP
	addr, err = ParseSingleIPTemplate("{{ GetPrivateIP }}")
	if err != nil {
		return "", fmt.Errorf("unable to parse default advertise address: %v", err)
	}
	return net.JoinHostPort(addr, strconv.Itoa(defport)), nil
}

func ConfigAgentFlagSet() *flag.FlagSet {
	// c := DefaultConfig()
	cmdFlags := flag.NewFlagSet("agent flagset", flag.ContinueOnError)
	cmdFlags.Bool("server", false,
		"This node is running in server mode")
	return cmdFlags
}

// ParseSingleIPTemplate is used as a helper function to parse out a single IP
// address from a config parameter.
func ParseSingleIPTemplate(ipTmpl string) (string, error) {
	out, err := template.Parse(ipTmpl)
	if err != nil {
		return "", fmt.Errorf("unable to parse address template %q: %v", ipTmpl, err)
	}

	ips := strings.Split(out, " ")
	switch len(ips) {
	case 0:
		return "", errors.New("no addresses found, please configure one")
	case 1:
		return ips[0], nil
	default:
		return "", fmt.Errorf("multiple addresses found (%q), please configure one", out)
	}
}

// AddrParts returns the parts of the BindAddr that should be
// used to configure Serf.
func (c *AgentConfig) AddrParts(address string) (string, int, error) {
	checkAddr := address
START:
	_, _, err := net.SplitHostPort(checkAddr)
	if ae, ok := err.(*net.AddrError); ok && ae.Err == "missing port in address" {
		checkAddr = fmt.Sprintf("%s:%d", checkAddr, DefaultBindPort)
		goto START
	}
	if err != nil {
		return "", 0, err
	}
	// Get the address
	addr, err := net.ResolveTCPAddr("tcp", checkAddr)
	if err != nil {
		return "", 0, err
	}

	return addr.IP.String(), addr.Port, nil
}

// EncryptBytes returns the encryption key configured.
func (c *AgentConfig) EncryptBytes() ([]byte, error) {
	return base64.StdEncoding.DecodeString(c.EncryptKey)
}
