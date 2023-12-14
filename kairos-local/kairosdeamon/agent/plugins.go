package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/THPTUHA/kairos/pkg/logger"
	"github.com/THPTUHA/kairos/server/plugin"
	"github.com/kardianos/osext"
	"github.com/rs/zerolog/log"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type Plugins struct {
	Executors       map[string]plugin.Executor
	Clients         map[string]*plugin.Client
	ClientProtocols map[string]*plugin.ClientProtocol
	LogLevel        string
	NodeName        string
}

func (p *Plugins) DiscoverPlugins() error {
	p.Executors = make(map[string]plugin.Executor)
	p.Clients = make(map[string]*plugin.Client)
	p.ClientProtocols = make(map[string]*plugin.ClientProtocol)
	p.LogLevel = logrus.DebugLevel.String()
	pluginDir := filepath.Join("~", "Code", "myproject", "kairos")

	if viper.ConfigFileUsed() != "" {
		pluginDir = filepath.Join(filepath.Dir(viper.ConfigFileUsed()), "plugins")
	}

	log.Info().Msg("Plugin Dir " + pluginDir)

	executors, err := plugin.Discover("kairos-executor-*", pluginDir)
	if err != nil {
		return err
	}

	exePath, err := osext.Executable()
	if err != nil {
		logrus.WithError(err).Error("Error loading exe directory")
	} else {

		e, err := plugin.Discover("kairos-executor-*", filepath.Dir(exePath))
		if err != nil {
			return err
		}
		executors = append(executors, e...)
	}

	log.Debug().Msg(fmt.Sprintf("executors %+v:", executors))

	for _, file := range executors {

		pluginName, ok := getPluginName(file)
		if !ok {
			continue
		}

		p.PluginFactory(exec.Command(file), pluginName, plugin.ExecutorPluginName)

	}
	// p.PluginFactory(exec.Command("node", "/Users/nghiabadao/Code/myproject/kairos/server/plugin/kairos-executor-node/index.js"), "node", plugin.ExecutorPluginName)
	return nil
}

func getPluginName(file string) (string, bool) {
	base := path.Base(file)
	parts := strings.SplitN(base, "-", 3)
	if len(parts) != 3 {
		return "", false
	}

	name := strings.TrimSuffix(parts[2], ".exe")
	name = strings.TrimSuffix(name, ".sh")
	name = strings.TrimSuffix(name, ".ps1")
	name = strings.TrimSuffix(name, ".bash")
	return name, true
}

func (p *Plugins) PluginFactory(cmd *exec.Cmd, pluginName, pluginType string) (interface{}, error) {
	var config plugin.ClientConfig
	config.Cmd = cmd
	config.HandshakeConfig = plugin.Handshake
	config.Managed = true
	config.Plugins = plugin.PluginMap
	config.SyncStdout = os.Stdout
	config.SyncStderr = os.Stderr
	config.Logger = &logger.HCLogAdapter{Logger: logger.InitLogger(p.LogLevel, p.NodeName), LoggerName: "plugins"}

	switch pluginType {
	case plugin.ExecutorPluginName:
		config.AllowedProtocols = []plugin.Protocol{plugin.ProtocolGRPC}
	}

	client := plugin.NewClient(&config)

	rpcClient, err := client.Client()
	if err != nil {
		return nil, err
	}

	raw, err := rpcClient.Dispense(pluginType)
	if err != nil {
		return nil, err
	}

	p.Clients[pluginName] = client
	p.ClientProtocols[pluginName] = &rpcClient
	p.Executors[pluginName] = raw.(plugin.Executor)
	return raw, nil
}
