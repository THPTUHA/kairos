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

	"github.com/hashicorp/go-sockaddr/template"
	flag "github.com/spf13/pflag"
)

type AgentConfig struct {
	// NodeName is the name we register as. Defaults to hostname.
	NodeName string `mapstructure:"node-name"`

	// BindAddr is the address on which all of kairos's services will
	// be bound. If not specified, this defaults to the first private ip address.
	BindAddr string `mapstructure:"bind-addr"`

	// HTTPAddr is the address on the UI web server will
	// be bound. If not specified, this defaults to all interfaces.
	HTTPAddr string `mapstructure:"http-addr"`

	// DevMode is used for development purposes only and limits the
	// use of persistence or state.
	DevMode bool

	// debug|info|warn|error|fatal|panic
	LogLevel string `mapstructure:"log-level"`

	// Tags are used to attach key/value metadata to a node.
	Tags map[string]string `mapstructure:"tags"`

	// EncryptKey is the secret key to use for encrypting communication
	// traffic for Serf. The secret key must be exactly 32-bytes, base64
	// encoded.
	EncryptKey string `mapstructure:"encrypt"`

	// StartJoin is a list of addresses to attempt to join when the
	// agent starts. If Serf is unable to communicate with any of these
	// addresses, then the agent will error and exit.
	StartJoin []string `mapstructure:"join"`

	Port int `mapstructure:"port"`

	// DataDir is the directory to store our state in
	DataDir string `mapstructure:"data-dir"`

	// MailHost is the SMTP server host to use for email notifications.
	MailHost string `mapstructure:"mail-host"`

	// MailPort is the SMTP server port to use for email notifications.
	MailPort uint16 `mapstructure:"mail-port"`

	// MailUsername is the SMTP server username to use for email notifications.
	MailUsername string `mapstructure:"mail-username"`

	// MailPassword is the SMTP server password to use for email notifications.
	MailPassword string `mapstructure:"mail-password"`

	// MailFrom is the email sender to use for email notifications.
	MailFrom string `mapstructure:"mail-from"`

	// MailPayload is the email template body to use for email notifications.
	MailPayload string `mapstructure:"mail-payload"`

	// MailSubjectPrefix is the email subject prefix string to use for email notifications.
	MailSubjectPrefix string `mapstructure:"mail-subject-prefix"`
}

var ErrResolvingHost = errors.New("error resolving hostname")

const (
	DefaultBindPort int = 8946
	DefaultPort     int = 6868
)

func DefaultConfig() *AgentConfig {
	hostname, err := os.Hostname()
	if err != nil {
		log.Panic(err)
	}

	tags := map[string]string{}

	return &AgentConfig{
		NodeName: hostname,
		BindAddr: fmt.Sprintf("{{ GetPrivateIP }}:%d", DefaultBindPort),
		HTTPAddr: ":8080",
		LogLevel: "debug",
		Port:     DefaultPort,
		Tags:     tags,
		DataDir:  "kairos.data",
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
