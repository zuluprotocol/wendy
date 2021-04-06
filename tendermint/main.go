package main

import (
	"os"
	"path/filepath"

	"github.com/spf13/viper"
	cfg "github.com/tendermint/tendermint/config"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/privval"
	"github.com/tendermint/tendermint/proxy"

	"code.vegaprotocol.io/wendy/tendermint/app"
	nm "code.vegaprotocol.io/wendy/tendermint/node"
)

func newConfig(root string) *cfg.Config {
	viper.Set("home", root)
	viper.SetConfigName("config")
	viper.AddConfigPath(root)
	viper.AddConfigPath(filepath.Join(root, "config"))

	if err := viper.ReadInConfig(); err != nil {
		panic(err)
	}

	config := cfg.DefaultConfig()
	if err := viper.Unmarshal(config); err != nil {
		panic(err)
	}
	config.SetRoot(os.ExpandEnv(root))
	cfg.EnsureRoot(config.RootDir)

	return config
}

func main() {
	var root = "$HOME/.tendermint"
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	config := newConfig(root)
	nodeKey, err := p2p.LoadOrGenNodeKey(config.NodeKeyFile())
	if err != nil {
		panic(err)
	}

	filePV := privval.LoadOrGenFilePV(config.PrivValidatorKeyFile(), config.PrivValidatorStateFile())
	logger := log.NewTMLogger(log.NewSyncWriter(os.Stdout))

	app := app.New()

	node, err := nm.NewNode(
		config,
		filePV,
		nodeKey,
		proxy.NewLocalClientCreator(app),
		nm.DefaultGenesisDocProviderFunc(config),
		nm.DefaultDBProvider,
		nm.DefaultMetricsProvider(config.Instrumentation),
		log.NewFilter(logger,
			log.AllowInfoWith("module", "app"),
			log.AllowInfoWith("module", "main"),
			log.AllowInfoWith("module", "state"),
			log.AllowError(),
		),
	)
	if err != nil {
		panic(err)
	}

	node.Start()
	node.Wait()
}