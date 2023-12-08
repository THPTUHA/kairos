package agent

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/config"
	"github.com/hashicorp/go-sockaddr/template"
	flag "github.com/spf13/pflag"
)

type AgentConfig struct {
	NodeName string `mapstructure:"node-name"`
	BindAddr string `mapstructure:"bind-addr"`
	HTTPAddr string `mapstructure:"http-addr"`
	DevMode  bool
	LogLevel string            `mapstructure:"log-level"`
	Tags     map[string]string `mapstructure:"tags"`

	Port    int    `mapstructure:"port"`
	DataDir string `mapstructure:"data-dir"`

	MailHost          string `mapstructure:"mail-host"`
	MailPort          uint16 `mapstructure:"mail-port"`
	MailUsername      string `mapstructure:"mail-username"`
	MailPassword      string `mapstructure:"mail-password"`
	MailFrom          string `mapstructure:"mail-from"`
	MailPayload       string `mapstructure:"mail-payload"`
	MailSubjectPrefix string `mapstructure:"mail-subject-prefix"`
}

var ErrResolvingHost = errors.New("error resolving hostname")

func DefaultConfig(c *config.Configs) *AgentConfig {
	hostname, err := os.Hostname()
	if err != nil {
		log.Panic(err)
	}

	tags := map[string]string{}

	return &AgentConfig{
		NodeName: hostname,
		BindAddr: fmt.Sprintf("{{ GetPrivateIP }}:%d", c.AgentDefaultPort),
		HTTPAddr: fmt.Sprintf(":%d", c.AgentHTTPAddrPort),
		LogLevel: "debug",
		Tags:     tags,
		DataDir:  c.AgentDataDir,
	}

	// return &AgentConfig{
	// 	NodeName: hostname,
	// 	BindAddr: fmt.Sprintf("{{ GetPrivateIP }}:%d", 8900),
	// 	HTTPAddr: ":8081",
	// 	LogLevel: "debug",
	// 	Port:     8901,
	// 	Tags:     tags,
	// 	DataDir:  "kairos.data",
	// }
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
	cmdFlags := flag.NewFlagSet("agent flagset", flag.ContinueOnError)
	cmdFlags.Bool("server", false,
		"This node is running in server mode")
	return cmdFlags
}

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
