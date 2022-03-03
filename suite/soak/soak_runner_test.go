package soak_runner

import (
	"fmt"
	"os"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/smartcontractkit/helmenv/environment"
	"github.com/smartcontractkit/helmenv/tools"
	"github.com/smartcontractkit/integrations-framework/config"
	"github.com/smartcontractkit/integrations-framework/utils"
	"github.com/stretchr/testify/require"
)

func TestSoakOCR(t *testing.T) {
	t.Parallel()
	utils.LoadConfigs(utils.ProjectRoot)

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	env, err := environment.DeployLongTestEnvironment(
		environment.NewChainlinkConfig(environment.ChainlinkReplicas(6, nil), "chainlink-soak"),
		// "git://github.com/smartcontractkit/integrations-framework.git", // The repo to pull your tests from
		"git://github.com/protofire/integrations-framework.git", // The repo to pull your tests from
		"soakRunner_celo2",               // The branch of the above repo to pull tests from
		"./suite/soak/tests", // The path of test files
		"@soak-ocr",          // REGEX name of the specific test you want to run
		config.ProjectFrameworkSettings.RemoteSlackWebhook,
		tools.ChartsRoot,
	)
	require.NoError(t, err)
	require.NotNil(t, env)
	log.Info().Str("Namespace", env.Namespace).
		Str("Environment File", fmt.Sprintf("%s.%s", env.Namespace, "yaml")).
		Msg("Soak Test Successfully Launched. Save the environment file to collect logs when test is done.")
}
