package main

import (
	"github.com/smartcontractkit/helmenv/environment"
	"github.com/smartcontractkit/helmenv/tools"
	"github.com/smartcontractkit/integrations-framework/actions"
	"github.com/smartcontractkit/integrations-framework/client"
	"github.com/smartcontractkit/integrations-framework/contracts"
)

var (
	err               error
	env               *environment.Environment
	networks          *client.Networks
	contractDeployer  contracts.ContractDeployer
	linkTokenContract contracts.LinkToken
	chainlinkNodes    []client.Chainlink
)

func main() {
	ocrInstances := make([]contracts.OffchainAggregator, 1)
	env, err = environment.DeployOrLoadEnvironment(
		environment.NewChainlinkConfig(environment.ChainlinkReplicas(6, nil)),
		tools.ChartsRoot,
	)
	err = env.ConnectAll()
	networkRegistry := client.NewNetworkRegistry()
	networks, err = networkRegistry.GetNetworks(env)
	contractDeployer, err = contracts.NewContractDeployer(networks.Default)
	chainlinkNodes, err = client.NewChainlinkClients(env)
	networks.Default.ParallelTransactions(true)
	linkTokenContract, err = contractDeployer.DeployLinkTokenContract()
	actions.FundNodes(networks, chainlinkNodes)
	actions.DeployOCRContracts(ocrInstances, linkTokenContract, contractDeployer, chainlinkNodes, networks)
}
