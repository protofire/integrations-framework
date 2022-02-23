package smoke

//revive:disable:dot-imports
import (
	"context"
	"github.com/smartcontractkit/helmenv/tools"

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
			//envConfig := make(map[string]interface{})
			//networkConfig := make(map[string]interface{})
			//nodeConfig := make(map[string]interface{})
			//envConfig["eth_url"] = "wss://alfajores-forno.celo-testnet.org/ws"
			//envConfig["eth_chain_id"] = "44787"
			//envConfig["eth_min_gas_price_wei"] = "100000000"
			//nodeConfig["image"] = map[string]interface{}{
			//	"image" : "celo-chainlink",
			//	"version": "v1.1.0",
			//}
			//
			//networkConfig["chainlink"] = nodeConfig
			//networkConfig["env"] = envConfig

			chainlinkConfig := environment.NewChainlinkConfig(
				environment.ChainlinkReplicas(6, nil),
			)

			env, err = environment.DeployOrLoadEnvironment(
				chainlinkConfig,
				tools.ChartsRoot,
			)
			Expect(err).ShouldNot(HaveOccurred(), "Environment deployment shouldn't fail")
			err = env.ConnectAll()
			Expect(err).ShouldNot(HaveOccurred(), "Connecting to all nodes shouldn't fail")
		})

		By("Connecting to launched resources", func() {
			// Load Networks
			networkRegistry := client.NewNetworkRegistry()
			var err error
			networks, err = networkRegistry.GetNetworks(env)
			Expect(err).ShouldNot(HaveOccurred(), "Connecting to blockchain nodes shouldn't fail")
			contractDeployer, err = contracts.NewContractDeployer(networks.Default)

			Expect(err).ShouldNot(HaveOccurred(), "Deploying contracts shouldn't fail")

			chainlinkNodes, err = client.ConnectChainlinkNodes(env)
			Expect(err).ShouldNot(HaveOccurred(), "Connecting to chainlink nodes shouldn't fail")
			mockserver, err = client.ConnectMockServer(env)
			Expect(err).ShouldNot(HaveOccurred(), "Creating mockserver clients shouldn't fail")

			networks.Default.ParallelTransactions(true)
			Expect(err).ShouldNot(HaveOccurred())

			linkTokenContract, err = contractDeployer.DeployLinkTokenContract()
			Expect(err).ShouldNot(HaveOccurred(), "Deploying Link Token Contract shouldn't fail")
		})

		By("Funding Chainlink nodes", func() {
			err = actions.FundChainlinkNodes(chainlinkNodes, networks.Default, big.NewFloat(.01))
			Expect(err).ShouldNot(HaveOccurred())
		})

		By("Deploying OCR contracts", func() {
			ocrInstances = actions.DeployOCRContracts(1, linkTokenContract, contractDeployer, chainlinkNodes, networks)
			err = networks.Default.WaitForEvents()
			Expect(err).ShouldNot(HaveOccurred())
		})
	})

	Describe("With a single OCR contract", func() {
		It("performs two rounds", func() {

			By("Setting adapter responses", actions.SetAllAdapterResponsesToTheSameValue(5, ocrInstances, chainlinkNodes, mockserver))
			By("Creating OCR jobs", actions.CreateOCRJobs(ocrInstances, chainlinkNodes, mockserver))

			By("Starting new round", actions.StartNewRound(1, ocrInstances, networks))

			answer, err := ocrInstances[0].GetLatestAnswer(context.Background())
			Expect(err).ShouldNot(HaveOccurred(), "Getting latest answer from OCR contract shouldn't fail")
			Expect(answer.Int64()).Should(Equal(int64(5)), "Expected latest answer from OCR contract to be 5 but got %d", answer.Int64())

			By("setting adapter responses", actions.SetAllAdapterResponsesToTheSameValue(10, ocrInstances, chainlinkNodes, mockserver))
			By("starting new round", actions.StartNewRound(2, ocrInstances, networks))

			answer, err = ocrInstances[0].GetLatestAnswer(context.Background())
			Expect(err).ShouldNot(HaveOccurred())
			Expect(answer.Int64()).Should(Equal(int64(10)), "Expected latest answer from OCR contract to be 10 but got %d", answer.Int64())
		})
	})

	AfterEach(func() {
		By("Printing gas stats", func() {
			networks.Default.GasStats().PrintStats()
		})
		By("Tearing down the environment", func() {
			err = actions.TeardownSuite(env, networks, utils.ProjectRoot, nil)
			Expect(err).ShouldNot(HaveOccurred(), "Environment teardown shouldn't fail")
		})
	})
})
