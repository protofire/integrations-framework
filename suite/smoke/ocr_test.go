package smoke

//revive:disable:dot-imports
import (
	"context"
	"fmt"
	"github.com/smartcontractkit/helmenv/tools"
	"log"
	"math/big"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/smartcontractkit/helmenv/environment"
	"github.com/smartcontractkit/integrations-framework/actions"
	"github.com/smartcontractkit/integrations-framework/client"
	"github.com/smartcontractkit/integrations-framework/contracts"
	"github.com/smartcontractkit/integrations-framework/utils"
)

var _ = FDescribe("OCR Feed @ocr", func() {
	var (
		err               error
		env               *environment.Environment
		networks          *client.Networks
		contractDeployer  contracts.ContractDeployer
		linkTokenContract contracts.LinkToken
		chainlinkNodes    []client.Chainlink
		mockserver        *client.MockserverClient
		ocrInstances      []contracts.OffchainAggregator
	)

	BeforeEach(func() {
		By("Deploying the environment", func() {
			envConfig := make(map[string]interface{})
			networkConfig := make(map[string]interface{})
			envConfig["eth_url"] = "wss://alfajores-forno.celo-testnet.org/ws"
			envConfig["eth_chain_id"] = "44787"
			envConfig["chainlink_image"] = "celo-chainlink"
			envConfig["chainlink_version"] = "latest"

			networkConfig["env"] = envConfig


			chainlinkConfig := environment.NewChainlinkConfig(
				environment.ChainlinkReplicas(6, networkConfig),
			)
			//chainlinkConfig.Namespace = "chainlink-mpd5q"
			env, err = environment.DeployOrLoadEnvironment(
				chainlinkConfig,
				tools.ChartsRoot,
			)
			Expect(err).ShouldNot(HaveOccurred())
			err = env.ConnectAll()
			Expect(err).ShouldNot(HaveOccurred())
		})

		By("Connecting to launched resources", func() {
			fmt.Printf("WS-RPC : %+v",env.Config.Charts["geth"])
			//.ChartConnections["geth_0_geth-network"].RemotePorts["ws-rpc"]
			// Load Networks
			networkRegistry := client.NewNetworkRegistry()
			var err error
			networks, err = networkRegistry.GetNetworks(env)
			if err != nil {
				log.Fatalln("Error found here: ", err)
			}
			Expect(err).ShouldNot(HaveOccurred())
			contractDeployer, err = contracts.NewContractDeployer(networks.Default)
			Expect(err).ShouldNot(HaveOccurred())

			chainlinkNodes, err = client.ConnectChainlinkNodes(env)
			Expect(err).ShouldNot(HaveOccurred())
			mockserver, err = client.ConnectMockServer(env)
			Expect(err).ShouldNot(HaveOccurred())

			networks.Default.ParallelTransactions(true)
			Expect(err).ShouldNot(HaveOccurred())

			linkTokenContract, err = contractDeployer.DeployLinkTokenContract()
			Expect(err).ShouldNot(HaveOccurred())
		})

		By("Funding Chainlink nodes", func() {
			err = actions.FundChainlinkNodes(chainlinkNodes, networks.Default, big.NewFloat(.01))
			Expect(err).ShouldNot(HaveOccurred())
		})

		By("Deploying OCR contracts", func() {
			ocrInstances = actions.DeployOCRContracts(1, linkTokenContract, contractDeployer, chainlinkNodes, networks)
			// Sending OCR jobs and start running them happens a lot more quickly
			// than the process of deploying OCR contracts
			// Hotfix
			err = networks.Default.WaitForEvents()
			Expect(err).ShouldNot(HaveOccurred())
		})

		By("Creating OCR jobs", actions.CreateOCRJobs(ocrInstances, chainlinkNodes, mockserver))
	})

	Describe("With a single OCR contract", func() {
		It("performs two rounds", func() {
			By("setting adapter responses", actions.SetAllAdapterResponses(5, ocrInstances, chainlinkNodes, mockserver))
			By("starting new round", actions.StartNewRound(1, ocrInstances, networks))

			answer, err := ocrInstances[0].GetLatestAnswer(context.Background())
			Expect(err).ShouldNot(HaveOccurred())
			Expect(answer.Int64()).Should(Equal(int64(5)), "latest answer from OCR is not as expected")

			By("setting adapter responses", actions.SetAllAdapterResponses(10, ocrInstances, chainlinkNodes, mockserver))
			By("starting new round", actions.StartNewRound(2, ocrInstances, networks))

			answer, err = ocrInstances[0].GetLatestAnswer(context.Background())
			Expect(err).ShouldNot(HaveOccurred())
			Expect(answer.Int64()).Should(Equal(int64(10)), "latest answer from OCR is not as expected")
		})
	})

	AfterEach(func() {
		By("Printing gas stats", func() {
			networks.Default.GasStats().PrintStats()
		})
		By("Tearing down the environment", func() {
			err = actions.TeardownSuite(env, networks, utils.ProjectRoot)
			Expect(err).ShouldNot(HaveOccurred())
		})
	})
})
