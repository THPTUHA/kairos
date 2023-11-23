package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/THPTUHA/kairos/pkg/logger"
	kplugin "github.com/THPTUHA/kairos/server/plugin"
	"github.com/hashicorp/go-plugin"
	"github.com/kardianos/osext"
	"github.com/rs/zerolog/log"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type Plugins struct {
	Processors map[string]Processor
	Executors  map[string]kplugin.Executor
	LogLevel   string
	NodeName   string
}

func (p *Plugins) DiscoverPlugins() error {
	p.Processors = make(map[string]Processor)
	p.Executors = make(map[string]kplugin.Executor)
	pluginDir := filepath.Join("~", "Code", "myproject", "kairos")

	if viper.ConfigFileUsed() != "" {
		pluginDir = filepath.Join(filepath.Dir(viper.ConfigFileUsed()), "plugins")
	}

	log.Info().Msg("Plugin Dir " + pluginDir)

	processors, err := plugin.Discover("kairos-processor-*", pluginDir)
	if err != nil {
		return err
	}

	executors, err := plugin.Discover("kairos-executor-*", pluginDir)
	if err != nil {
		return err
	}

	exePath, err := osext.Executable()
	if err != nil {
		logrus.WithError(err).Error("Error loading exe directory")
	} else {
		p, err := plugin.Discover("kairos-processor-*", filepath.Dir(exePath))
		if err != nil {
			return err
		}
		processors = append(processors, p...)
		e, err := plugin.Discover("kairos-executor-*", filepath.Dir(exePath))
		if err != nil {
			return err
		}
		executors = append(executors, e...)
	}

	log.Debug().Msg(fmt.Sprintf("executors %+v:", executors))
	for _, file := range processors {

		pluginName, ok := getPluginName(file)
		if !ok {
			continue
		}

		raw, err := p.pluginFactory(file, kplugin.ProcessorPluginName)
		if err != nil {
			return err
		}
		p.Processors[pluginName] = raw.(Processor)
	}

	for _, file := range executors {

		pluginName, ok := getPluginName(file)
		if !ok {
			continue
		}

		raw, err := p.pluginFactory(file, kplugin.ExecutorPluginName)
		if err != nil {
			return err
		}
		p.Executors[pluginName] = raw.(kplugin.Executor)
	}

	return nil
}

func getPluginName(file string) (string, bool) {
	base := path.Base(file)
	parts := strings.SplitN(base, "-", 3)
	if len(parts) != 3 {
		return "", false
	}

	name := strings.TrimSuffix(parts[2], ".exe")
	return name, true
}

func (p *Plugins) pluginFactory(path string, pluginType string) (interface{}, error) {
	var config plugin.ClientConfig
	config.Cmd = exec.Command(path)
	config.HandshakeConfig = kplugin.Handshake
	config.Managed = true
	config.Plugins = kplugin.PluginMap
	config.SyncStdout = os.Stdout
	config.SyncStderr = os.Stderr
	config.Logger = &logger.HCLogAdapter{Logger: logger.InitLogger(p.LogLevel, p.NodeName), LoggerName: "plugins"}

	switch pluginType {
	case kplugin.ProcessorPluginName:
		config.AllowedProtocols = []plugin.Protocol{plugin.ProtocolNetRPC}
	case kplugin.ExecutorPluginName:
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

	return raw, nil
}
