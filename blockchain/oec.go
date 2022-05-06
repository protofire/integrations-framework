package blockchain

import (
	"context"
	"fmt"
	"math/big"
	"net/url"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/rs/zerolog/log"
	"github.com/smartcontractkit/chainlink-testing-framework/config"
	"github.com/smartcontractkit/chainlink-testing-framework/utils"
)

// Handles specific issues with the Oec EVM chain: https://okexchain-docs.readthedocs.io/en/latest/developers/quick-start.html

// OecMultinodeClient represents a multi-node, EVM compatible client for the Oec network
type OecMultinodeClient struct {
	*EthereumMultinodeClient
}

// OecClient represents a single node, EVM compatible client for the Oec network
type OecClient struct {
	*EthereumClient
}

// NewOecClient returns an instantiated instance of the Oec client that has connected to the server
func NewOecClient(networkSettings *config.ETHNetwork) (EVMClient, error) {
	client, err := NewEthereumClient(networkSettings)
	log.Info().Str("Network Name", client.GetNetworkName()).Msg("Using custom Oec client")
	return &OecClient{client.(*EthereumClient)}, err
}

// NewOecMultinodeClient returns an instantiated instance of all Oec clients connected to all nodes
func NewOecMultiNodeClient(
	_ string,
	networkConfig map[string]interface{},
	urls []*url.URL,
) (EVMClient, error) {
	networkSettings := &config.ETHNetwork{}
	err := UnmarshalNetworkConfig(networkConfig, networkSettings)
	if err != nil {
		return nil, err
	}
	log.Info().
		Interface("URLs", networkSettings.URLs).
		Msg("Connecting multi-node client")

	multiNodeClient := &EthereumMultinodeClient{}
	for _, envURL := range urls {
		networkSettings.URLs = append(networkSettings.URLs, envURL.String())
	}
	for idx, networkURL := range networkSettings.URLs {
		networkSettings.URL = networkURL
		ec, err := NewOecClient(networkSettings)
		if err != nil {
			return nil, err
		}
		ec.SetID(idx)
		multiNodeClient.Clients = append(multiNodeClient.Clients, ec)
	}
	multiNodeClient.DefaultClient = multiNodeClient.Clients[0]
	return &OecMultinodeClient{multiNodeClient}, nil
}

// Fund sends some ETH to an address using the default wallet
func (m *OecClient) Fund(toAddress string, amount *big.Float) error {
	privateKey, err := crypto.HexToECDSA(m.DefaultWallet.PrivateKey())
	to := common.HexToAddress(toAddress)
	if err != nil {
		return fmt.Errorf("invalid private key: %v", err)
	}
	// Oec uses legacy transactions and gas estimations, is behind London fork as of 04/27/2022
	suggestedGasPrice, err := m.Client.SuggestGasPrice(context.Background())
	if err != nil {
		return err
	}

	// Bump gas price
	gasPriceBuffer := big.NewInt(0).SetUint64(m.NetworkConfig.GasEstimationBuffer)
	suggestedGasPrice.Add(suggestedGasPrice, gasPriceBuffer)

	nonce, err := m.GetNonce(context.Background(), common.HexToAddress(m.DefaultWallet.Address()))
	if err != nil {
		return err
	}

	tx, err := types.SignNewTx(privateKey, types.LatestSignerForChainID(m.GetChainID()), &types.LegacyTx{
		Nonce:    nonce,
		To:       &to,
		Value:    utils.EtherToWei(amount),
		GasPrice: suggestedGasPrice,
		Gas:      22000,
	})
	if err != nil {
		return err
	}

	log.Info().
		Str("Token", "OKT").
		Str("From", m.DefaultWallet.Address()).
		Str("To", toAddress).
		Str("Amount", amount.String()).
		Msg("Funding Address")
	if err := m.Client.SendTransaction(context.Background(), tx); err != nil {
		return err
	}

	return m.ProcessTransaction(tx)
}

// DeployContract acts as a general contract deployment tool to an EVM chain
func (m *OecClient) DeployContract(
	contractName string,
	deployer ContractDeployer,
) (*common.Address, *types.Transaction, interface{}, error) {
	opts, err := m.TransactionOpts(m.DefaultWallet)
	if err != nil {
		return nil, nil, nil, err
	}

	// Oec uses legacy transactions and gas estimations, is behind London fork as of 04/21/2022
	suggestedGasPrice, err := m.Client.SuggestGasPrice(context.Background())
	if err != nil {
		return nil, nil, nil, err
	}

	// Bump gas price
	gasPriceBuffer := big.NewInt(0).SetUint64(m.NetworkConfig.GasEstimationBuffer)
	suggestedGasPrice.Add(suggestedGasPrice, gasPriceBuffer)

	opts.GasPrice = suggestedGasPrice

	contractAddress, transaction, contractInstance, err := deployer(opts, m.Client)
	if err != nil {
		return nil, nil, nil, err
	}

	if err := m.ProcessTransaction(transaction); err != nil {
		return nil, nil, nil, err
	}

	log.Info().
		Str("Contract Address", contractAddress.Hex()).
		Str("Contract Name", contractName).
		Str("From", m.DefaultWallet.Address()).
		Str("Total Gas Cost (OKT)", utils.WeiToEther(transaction.Cost()).String()).
		Str("Network Name", m.NetworkConfig.Name).
		Msg("Deployed contract")
	return &contractAddress, transaction, contractInstance, err
}
