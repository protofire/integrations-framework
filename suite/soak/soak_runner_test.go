package soak_runner

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/smartcontractkit/helmenv/environment"
	"github.com/smartcontractkit/helmenv/tools"
	"github.com/smartcontractkit/integrations-framework/actions"
	"github.com/smartcontractkit/integrations-framework/config"
	"github.com/smartcontractkit/integrations-framework/utils"
	"github.com/stretchr/testify/require"
)

func TestSoakOCR(t *testing.T) {
	t.Parallel()
	actions.LoadConfigs(utils.ProjectRoot)
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	remoteConfig, err := config.ReadWriteRemoteRunnerConfig()
	require.NoError(t, err)

	config := environment.NewChainlinkConfig(
		environment.ChainlinkReplicas(6, config.ChainlinkVals()),
		"chainlink-soak",
		config.GethNetworks()...,
	)

	envconfig.Process("", config)

	env, err := environment.DeployLongTestEnvironment(
		config,
		tools.ChartsRoot,
		remoteConfig.TestRegex,    // Name of the test to run
		remoteConfig.SlackAPIKey,  // API key to use to upload artifacts to slack
		remoteConfig.SlackChannel, // Slack Channel to upload test artifacts to
		remoteConfig.SlackUserID,  // Slack user to notify on completion
		filepath.Join(utils.ProjectRoot, "framework.yaml"), // Path of the framework config
		filepath.Join(utils.ProjectRoot, "networks.yaml"),  // Path to the networks config
		"", // Path to the executable test file
	)
	require.NoError(t, err)
	require.NotNil(t, env)
	log.Info().Str("Namespace", env.Namespace).
		Str("Environment File", fmt.Sprintf("%s.%s", env.Namespace, "yaml")).
		Msg("Soak Test Successfully Launched. Save the environment file to collect logs when test is done.")
}
