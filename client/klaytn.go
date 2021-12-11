package client

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/klaytn/klaytn/accounts/abi"
	"github.com/pkg/errors"

	klaytnContracts "github.com/smartcontractkit/integrations-framework/contracts/klaytn"
	"github.com/smartcontractkit/integrations-framework/klaytnextended"

	ethereum "github.com/klaytn/klaytn"
	"github.com/klaytn/klaytn/accounts/abi/bind"
	"github.com/klaytn/klaytn/blockchain/types"
	ethclient "github.com/klaytn/klaytn/client"
	"github.com/klaytn/klaytn/common"
	"github.com/klaytn/klaytn/crypto"
	"github.com/rs/zerolog/log"
)

type KlaytnBlock struct {
	*types.Block
}

func (e *KlaytnBlock) GetHash() HashInterface {
	return e.Hash()
}

// KlaytnClients wraps the client and the BlockChain network to interact with an Klaytn EVM based Blockchain with multiple nodes
type KlaytnClients struct {
	DefaultClient *KlaytnClient
	Clients       []*KlaytnClient
}

// GetNetworkName gets the ID of the chain that the clients are connected to
func (e *KlaytnClients) GetNetworkName() string {
	return e.DefaultClient.GetNetworkName()
}

// GetID gets client ID, node number it's connected to
func (e *KlaytnClients) GetID() int {
	return e.DefaultClient.ID
}

// GasStats gets gas stats instance
func (e *KlaytnClients) GasStats() *GasStats {
	return e.DefaultClient.gasStats
}

// SetDefaultClient sets default client to perform calls to the network
func (e *KlaytnClients) SetDefaultClient(clientID int) error {
	if clientID > len(e.Clients) {
		return fmt.Errorf("client for node %d not found", clientID)
	}
	e.DefaultClient = e.Clients[clientID]
	return nil
}

// GetClients gets clients for all nodes connected
func (e *KlaytnClients) GetClients() []BlockchainClient {
	cl := make([]BlockchainClient, 0)
	for _, c := range e.Clients {
		cl = append(cl, c)
	}
	return cl
}

// SetID sets client ID (node)
func (e *KlaytnClients) SetID(id int) {
	e.DefaultClient.SetID(id)
}

// BlockNumber gets block number
func (e *KlaytnClients) BlockNumber(ctx context.Context) (uint64, error) {
	return e.DefaultClient.BlockNumber(ctx)
}

// HeaderTimestampByNumber gets header timestamp by number
func (e *KlaytnClients) HeaderTimestampByNumber(ctx context.Context, bn *big.Int) (int64, error) {
	return e.DefaultClient.HeaderTimestampByNumber(ctx, bn)
}

// HeaderHashByNumber gets header hash by block number
func (e *KlaytnClients) HeaderHashByNumber(ctx context.Context, bn *big.Int) (string, error) {
	return e.DefaultClient.HeaderHashByNumber(ctx, bn)
}

// Get gets default client as an interface{}
func (e *KlaytnClients) Get() interface{} {
	return e.DefaultClient
}

// CalculateTxGas calculates tx gas cost accordingly gas used plus buffer, converts it to big.Float for funding
func (e *KlaytnClients) CalculateTxGas(gasUsedValue *big.Int) (*big.Float, error) {
	return e.DefaultClient.CalculateTxGas(gasUsedValue)
}

// Fund funds a specified address with LINK token and or ETH from the given wallet
func (e *KlaytnClients) Fund(fromWallet BlockchainWallet, toAddress string, nativeAmount, linkAmount *big.Float) error {
	return e.DefaultClient.Fund(fromWallet, toAddress, nativeAmount, linkAmount)
}

// ParallelTransactions when enabled, sends the transaction without waiting for transaction confirmations. The hashes
// are then stored within the client and confirmations can be waited on by calling WaitForEvents.
// When disabled, the minimum confirmations are waited on when the transaction is sent, so parallelisation is disabled.
func (e *KlaytnClients) ParallelTransactions(enabled bool) {
	for _, c := range e.Clients {
		c.ParallelTransactions(enabled)
	}
}

// Close tears down the all the clients
func (e *KlaytnClients) Close() error {
	for _, c := range e.Clients {
		if err := c.Close(); err != nil {
			return err
		}
	}
	return nil
}

// AddHeaderEventSubscription adds a new header subscriber within the client to receive new headers
func (e *KlaytnClients) AddHeaderEventSubscription(key string, subscriber HeaderEventSubscription) {
	for _, c := range e.Clients {
		c.AddHeaderEventSubscription(key, subscriber)
	}
}

// DeleteHeaderEventSubscription removes a header subscriber from the map
func (e *KlaytnClients) DeleteHeaderEventSubscription(key string) {
	for _, c := range e.Clients {
		c.DeleteHeaderEventSubscription(key)
	}
}

// WaitForEvents is a blocking function that waits for all event subscriptions for all clients
func (e *KlaytnClients) WaitForEvents() error {
	g := errgroup.Group{}
	for _, c := range e.Clients {
		c := c
		g.Go(func() error {
			return c.WaitForEvents()
		})
	}
	return g.Wait()
}

// KlaytnClient wraps the client and the BlockChain network to interact with an EVM based Blockchain
type KlaytnClient struct {
	ID                  int
	Client              *ethclient.Client
	Network             BlockchainNetwork
	BorrowNonces        bool
	NonceMu             *sync.Mutex
	Nonces              map[string]uint64
	txQueue             chan common.Hash
	headerSubscriptions map[string]HeaderEventSubscription
	mutex               *sync.Mutex
	queueTransactions   bool
	gasStats            *GasStats
	doneChan            chan struct{}
}

// GetID gets client ID, node number it's connected to
func (e *KlaytnClient) GetID() int {
	return e.ID
}

// SetDefaultClient not used, only applicable to KlaytnClients
func (e *KlaytnClient) SetDefaultClient(_ int) error {
	return nil
}

// GetClients not used, only applicable to KlaytnClients
func (e *KlaytnClient) GetClients() []BlockchainClient {
	return []BlockchainClient{e}
}

// SuggestGasPrice gets suggested gas price
func (e *KlaytnClient) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	gasPrice, err := e.Client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, err
	}
	return gasPrice, nil
}

// SetID sets client id, useful for multi-node networks
func (e *KlaytnClient) SetID(id int) {
	e.ID = id
}

// BlockNumber gets latest block number
func (e *KlaytnClient) BlockNumber(ctx context.Context) (uint64, error) {
	bn, err := e.Client.BlockByNumber(ctx, nil)
	if err != nil {
		return 0, err
	}
	return bn.NumberU64(), nil
}

// HeaderHashByNumber gets header hash by block number
func (e *KlaytnClient) HeaderHashByNumber(ctx context.Context, bn *big.Int) (string, error) {
	h, err := e.Client.HeaderByNumber(ctx, bn)
	if err != nil {
		return "", err
	}
	return h.Hash().String(), nil
}

// HeaderTimestampByNumber gets header timestamp by number
func (e *KlaytnClient) HeaderTimestampByNumber(ctx context.Context, bn *big.Int) (int64, error) {
	h, err := e.Client.HeaderByNumber(ctx, bn)
	if err != nil {
		return 0, err
	}
	return h.Time.Int64(), nil
}

// KlaytnContractDeployer acts as a go-between function for general contract deployment
type KlaytnContractDeployer func(auth *bind.TransactOpts, backend bind.ContractBackend) (
	common.Address,
	*types.Transaction,
	interface{},
	error,
)

// NewKlaytnClient returns an instantiated instance of the Klaytn client that has connected to the server
func NewKlaytnClient(network BlockchainNetwork) (*KlaytnClient, error) {
	cl, err := ethclient.Dial(network.LocalURL())
	if err != nil {
		return nil, err
	}

	ec := &KlaytnClient{
		Network:             network,
		Client:              cl,
		BorrowNonces:        true,
		NonceMu:             &sync.Mutex{},
		Nonces:              make(map[string]uint64),
		txQueue:             make(chan common.Hash, 64), // Max buffer of 64 tx
		headerSubscriptions: map[string]HeaderEventSubscription{},
		mutex:               &sync.Mutex{},
		queueTransactions:   false,
		doneChan:            make(chan struct{}),
	}
	ec.gasStats = NewGasStats(ec.ID)
	go ec.newHeadersLoop()
	return ec, nil
}

// NewKlaytnClients returns an instantiated instance of all Klaytn client connected to all nodes
func NewKlaytnClients(network BlockchainNetwork) (*KlaytnClients, error) {
	ecl := &KlaytnClients{Clients: make([]*KlaytnClient, 0)}
	for idx, url := range network.URLs() {
		network.SetLocalURL(url)
		ec, err := NewKlaytnClient(network)
		if err != nil {
			return nil, err
		}
		ec.SetID(idx)
		ecl.Clients = append(ecl.Clients, ec)
	}
	ecl.DefaultClient = ecl.Clients[0]
	return ecl, nil
}

// GetNetworkName retrieves the ID of the network that the client interacts with
func (e *KlaytnClient) GetNetworkName() string {
	return e.Network.ID()
}

// Close tears down the current open Klaytn client
func (e *KlaytnClient) Close() error {
	e.doneChan <- struct{}{}
	e.Client.Close()
	return nil
}

// SuggestGasPrice gets suggested gas price
func (e *KlaytnClients) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	gasPrice, err := e.DefaultClient.SuggestGasPrice(ctx)
	if err != nil {
		return nil, err
	}
	return gasPrice, nil
}

// BorrowedNonces allows to handle nonces concurrently without requesting them every time
func (e *KlaytnClient) BorrowedNonces(n bool) {
	e.BorrowNonces = n
}

// GetNonce keep tracking of nonces per address, add last nonce for addr if the map is empty
func (e *KlaytnClient) GetNonce(ctx context.Context, addr common.Address) (uint64, error) {
	if e.BorrowNonces {
		e.NonceMu.Lock()
		defer e.NonceMu.Unlock()
		if _, ok := e.Nonces[addr.Hex()]; !ok {
			lastNonce, err := e.Client.PendingNonceAt(ctx, addr)
			if err != nil {
				return 0, err
			}
			e.Nonces[addr.Hex()] = lastNonce
			return lastNonce, nil
		}
		e.Nonces[addr.Hex()] += 1
		return e.Nonces[addr.Hex()], nil
	}
	lastNonce, err := e.Client.PendingNonceAt(ctx, addr)
	if err != nil {
		return 0, err
	}
	return lastNonce, nil
}

// Get returns the underlying client type to be used generically across the framework for switching
// network types
func (e *KlaytnClient) Get() interface{} {
	return e
}

// CalculateTxGas calculates tx gas cost accordingly gas used plus buffer, converts it to big.Float for funding
func (e *KlaytnClient) CalculateTxGas(gasUsed *big.Int) (*big.Float, error) {
	gasPrice, err := e.Client.SuggestGasPrice(context.Background()) // Wei
	if err != nil {
		return nil, err
	}
	buffer := big.NewInt(0).SetUint64(e.Network.Config().GasEstimationBuffer)
	gasUsedWithBuffer := gasUsed.Add(gasUsed, buffer)
	cost := big.NewFloat(0).SetInt(big.NewInt(1).Mul(gasPrice, gasUsedWithBuffer))
	costInEth := big.NewFloat(0).Quo(cost, OneEth)
	costInEthFloat, _ := costInEth.Float64()

	log.Debug().Float64("TX Gas cost", costInEthFloat).Msg("Estimated tx gas cost with buffer")
	return costInEth, nil
}

// GasStats gets gas stats instance
func (e *KlaytnClient) GasStats() *GasStats {
	return e.gasStats
}

// ParallelTransactions when enabled, sends the transaction without waiting for transaction confirmations. The hashes
// are then stored within the client and confirmations can be waited on by calling WaitForEvents.
// When disabled, the minimum confirmations are waited on when the transaction is sent, so parallelisation is disabled.
func (e *KlaytnClient) ParallelTransactions(enabled bool) {
	e.queueTransactions = enabled
}

// Fund funds a specified address with LINK token and or ETH from the given wallet
func (e *KlaytnClient) Fund(
	fromWallet BlockchainWallet,
	toAddress string,
	ethAmount, linkAmount *big.Float,
) error {
	ethAddress := common.HexToAddress(toAddress)
	// Send ETH if not 0
	if ethAmount != nil && big.NewFloat(0).Cmp(ethAmount) != 0 {
		weiValue := big.NewFloat(1).Mul(OneEth, ethAmount) // Convert ETH -> Wei
		log.Info().
			Str("Token", "ETH").
			Str("From", fromWallet.Address()).
			Str("To", toAddress).
			Str("Amount", ethAmount.String()).
			Msg("Funding Address")
		_, err := e.SendTransaction(fromWallet, ethAddress, weiValue)
		if err != nil {
			return err
		}
	}

	// Send LINK if not 0
	if linkAmount != nil && big.NewFloat(0).Cmp(linkAmount) != 0 {
		link := big.NewFloat(1).Mul(OneLINK, linkAmount)
		log.Info().
			Str("Token", "LINK").
			Str("From", fromWallet.Address()).
			Str("To", toAddress).
			Str("Amount", linkAmount.String()).
			Msg("Funding Address")
		linkAddress := common.HexToAddress(e.Network.Config().LinkTokenAddress)
		linkInstance, err := klaytnContracts.NewLinkToken(linkAddress, e.Client)
		if err != nil {
			return err
		}
		opts, err := e.TransactionOpts(fromWallet)
		if err != nil {
			return err
		}
		linkInt, _ := link.Int(nil)
		tx, err := linkInstance.Transfer(opts, ethAddress, linkInt)
		if err != nil {
			return err
		}
		return e.ProcessTransaction(tx) // TODO: LINK Transactions are either moving too slowly, or have multiple parts to them that breaks when trying to make them parallel
	}
	return nil
}

// SendTransaction sends a specified amount of ETH from a selected wallet to an address, and blocks until the
// transaction completes
func (e *KlaytnClient) SendTransaction(
	from BlockchainWallet,
	to common.Address,
	value *big.Float,
) (common.Hash, error) {
	weiValue, _ := value.Int(nil)
	privateKey, err := crypto.HexToECDSA(from.PrivateKey())
	if err != nil {
		return common.Hash{}, fmt.Errorf("invalid private key: %v", err)
	}
	suggestedGasPrice, err := e.Client.SuggestGasPrice(context.Background())
	if err != nil {
		return common.Hash{}, err
	}
	nonce, err := e.GetNonce(context.Background(), common.HexToAddress(from.Address()))
	if err != nil {
		return common.Hash{}, err
	}

	tx := types.NewTransaction(
		nonce,
		to,
		weiValue,
		21000,
		suggestedGasPrice,
		nil,
	)

	txSigned, err := types.SignTx(tx, types.NewEIP155Signer(e.Network.ChainID()), privateKey)
	if err != nil {
		return common.Hash{}, err
	}
	if err := e.Client.SendTransaction(context.Background(), txSigned); err != nil {
		return common.Hash{}, err
	}
	return txSigned.Hash(), e.ProcessTransaction(txSigned)
}

// ProcessTransaction will queue or wait on a transaction depending on whether parallel transactions are enabled
func (e *KlaytnClient) ProcessTransaction(tx *types.Transaction) error {
	var txConfirmer HeaderEventSubscription
	if e.Network.Config().MinimumConfirmations == 0 {
		txConfirmer = &KlaytnInstantConfirmations{}
	} else {
		txConfirmer = NewKlaytnTransactionConfirmer(e, tx, e.Network.Config().MinimumConfirmations)
	}

	e.AddHeaderEventSubscription(tx.Hash().String(), txConfirmer)

	if !e.queueTransactions || tx.Value().Cmp(big.NewInt(0)) == 0 { // For sequential transactions and contract calls
		defer e.DeleteHeaderEventSubscription(tx.Hash().String())
		return txConfirmer.Wait()
	}
	return nil
}

// DeployContract acts as a general contract deployment tool to an Klaytn chain
func (e *KlaytnClient) DeployContract(
	fromWallet BlockchainWallet,
	contractName string,
	deployer KlaytnContractDeployer,
) (*common.Address, *types.Transaction, interface{}, error) {
	opts, err := e.TransactionOpts(fromWallet)
	if err != nil {
		return nil, nil, nil, err
	}
	contractAddress, transaction, contractInstance, err := deployer(opts, e.Client)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := e.ProcessTransaction(transaction); err != nil {
		return nil, nil, nil, err
	}
	totalGasCostWeiFloat := big.NewFloat(1).SetInt(transaction.Cost())
	totalGasCostGwei := big.NewFloat(1).Quo(totalGasCostWeiFloat, OneGWei)

	log.Info().
		Str("Contract Address", contractAddress.Hex()).
		Str("Contract Name", contractName).
		Str("From", fromWallet.Address()).
		Str("Total Gas Cost (GWei)", totalGasCostGwei.String()).
		Str("Network", e.Network.ID()).
		Msg("Deployed contract")
	return &contractAddress, transaction, contractInstance, err
}

// TransactionOpts returns the base Tx options for 'transactions' that interact with a smart contract. Since most
// contract interactions in this framework are designed to happen through abigen calls, it's intentionally quite bare.
// abigen will handle gas estimation for us on the backend.
func (e *KlaytnClient) TransactionOpts(from BlockchainWallet) (*bind.TransactOpts, error) {
	privateKey, err := crypto.HexToECDSA(from.PrivateKey())
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %v", err)
	}
	opts, err := klaytnextended.NewKeyedTransactorWithChainID(privateKey, e.Network.ChainID())
	if err != nil {
		return nil, err
	}
	opts.From = common.HexToAddress(from.Address())
	opts.Context = context.Background()

	nonce, err := e.GetNonce(context.Background(), common.HexToAddress(from.Address()))
	if err != nil {
		return nil, err
	}
	opts.Nonce = big.NewInt(int64(nonce))

	return opts, nil
}

// WaitForEvents is a blocking function that waits for all event subscriptions that have been queued within the client.
func (e *KlaytnClient) WaitForEvents() error {
	queuedEvents := e.GetHeaderSubscriptions()
	g := errgroup.Group{}

	for subName, sub := range queuedEvents {
		subName := subName
		sub := sub
		g.Go(func() error {
			defer e.DeleteHeaderEventSubscription(subName)
			return sub.Wait()
		})
	}
	return g.Wait()
}

// GetHeaderSubscriptions returns a duplicate map of the queued transactions
func (e *KlaytnClient) GetHeaderSubscriptions() map[string]HeaderEventSubscription {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	newMap := map[string]HeaderEventSubscription{}
	for k, v := range e.headerSubscriptions {
		newMap[k] = v
	}
	return newMap
}

// AddHeaderEventSubscription adds a new header subscriber within the client to receive new headers
func (e *KlaytnClient) AddHeaderEventSubscription(key string, subscriber HeaderEventSubscription) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.headerSubscriptions[key] = subscriber
}

// DeleteHeaderEventSubscription removes a header subscriber from the map
func (e *KlaytnClient) DeleteHeaderEventSubscription(key string) {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	delete(e.headerSubscriptions, key)
}

func (e *KlaytnClient) newHeadersLoop() {
	for {
		if err := e.subscribeToNewHeaders(); err != nil {
			log.Error().
				Str("Network", e.Network.ID()).
				Msgf("Error while subscribing to headers: %v", err.Error())
			time.Sleep(time.Second)
			continue
		}
		break
	}
	log.Debug().Str("Network", e.Network.ID()).Msg("Stopped subscribing to new headers")
}

func (e *KlaytnClient) subscribeToNewHeaders() error {
	headerChannel := make(chan *types.Header)
	subscription, err := e.Client.SubscribeNewHead(context.Background(), headerChannel)
	if err != nil {
		return err
	}
	defer subscription.Unsubscribe()

	log.Info().Str("Network", e.Network.ID()).Msg("Subscribed to new block headers")

	for {
		select {
		case err := <-subscription.Err():
			return err
		case header := <-headerChannel:
			e.receiveHeader(header)
		case <-e.doneChan:
			return nil
		}
	}
}

func (e *KlaytnClient) receiveHeader(header *types.Header) {
	log.Debug().
		Str("Network", e.Network.ID()).
		Int("Node", e.ID).
		Str("Hash", header.Hash().String()).
		Str("Number", header.Number.String()).
		Msg("Received block header")

	subs := e.GetHeaderSubscriptions()
	block, err := e.Client.BlockByNumber(context.Background(), header.Number)
	if err != nil {
		log.Err(fmt.Errorf("error fetching block by number: %v", err))
	}

	KlaytnBlock := &KlaytnBlock{block}

	g := errgroup.Group{}
	for _, sub := range subs {
		sub := sub
		g.Go(func() error {
			return sub.ReceiveBlock(NodeBlock{NodeID: e.ID, BlockInterface: KlaytnBlock})
		})
	}
	if err := g.Wait(); err != nil {
		log.Err(fmt.Errorf("error on sending block to receivers: %v", err))
	}
}

func (e *KlaytnClient) isTxConfirmed(txHash common.Hash) (bool, error) {
	tx, isPending, err := e.Client.TransactionByHash(context.Background(), txHash)
	if err != nil {
		return !isPending, err
	}
	if !isPending {
		receipt, err := e.Client.TransactionReceipt(context.Background(), txHash)
		if err != nil {
			return !isPending, err
		}
		// TODO koteld: CumulativeGasUsed removed
		e.gasStats.AddClientTXData(TXGasData{
			TXHash:            txHash.String(),
			Value:             tx.Value().Uint64(),
			GasLimit:          tx.Gas(),
			GasUsed:           receipt.GasUsed,
			GasPrice:          tx.GasPrice().Uint64(),
			CumulativeGasUsed: receipt.CumulativeGasUsed,
		})
		if receipt.Status == 0 { // 0 indicates failure, 1 indicates success
			reason, err := e.errorReason(e.Client, tx, receipt)
			if err != nil {
				log.Warn().Str("TX Hash", txHash.Hex()).
					Str("To", tx.To().Hex()).
					Str("Hint", "We often see this with parallel transactions ending up with out-of-order nonces").
					Uint64("Nonce", tx.Nonce()).
					Msg("Transaction failed and was reverted! Unable to retrieve reason!")
				return false, err
			}
			log.Warn().Str("TX Hash", txHash.Hex()).
				Str("To", tx.To().Hex()).
				Str("Revert reason", reason).
				Msg("Transaction failed and was reverted!")
		}
	}
	return !isPending, err
}

// errorReason decodes tx revert reason
func (e *KlaytnClient) errorReason(
	b ethereum.ContractCaller,
	tx *types.Transaction,
	receipt *types.Receipt,
) (string, error) {
	chID, err := e.Client.NetworkID(context.Background())
	if err != nil {
		return "", err
	}
	from, err := types.Sender(types.NewEIP155Signer(chID), tx)
	if err != nil {
		return "", err
	}
	callMsg := ethereum.CallMsg{
		From:     from,
		To:       tx.To(),
		Gas:      tx.Gas(),
		GasPrice: tx.GasPrice(),
		Value:    tx.Value(),
		Data:     tx.Data(),
	}
	res, err := b.CallContract(context.Background(), callMsg, receipt.BlockNumber)
	if err != nil {
		return "", errors.Wrap(err, "CallContract")
	}
	return abi.UnpackRevert(res)
}

// KlaytnTransactionConfirmer is an implementation of HeaderEventSubscription that checks whether tx are confirmed
type KlaytnTransactionConfirmer struct {
	minConfirmations int
	confirmations    int
	eth              *KlaytnClient
	tx               *types.Transaction
	doneChan         chan struct{}
	context          context.Context
	cancel           context.CancelFunc
}

// NewKlaytnTransactionConfirmer returns a new instance of the transaction confirmer that waits for on-chain minimum
// confirmations
func NewKlaytnTransactionConfirmer(eth *KlaytnClient, tx *types.Transaction, minConfirmations int) *KlaytnTransactionConfirmer {
	ctx, ctxCancel := context.WithTimeout(context.Background(), eth.Network.Config().Timeout)
	tc := &KlaytnTransactionConfirmer{
		minConfirmations: minConfirmations,
		confirmations:    0,
		eth:              eth,
		tx:               tx,
		doneChan:         make(chan struct{}, 1),
		context:          ctx,
		cancel:           ctxCancel,
	}
	return tc
}

// ReceiveBlock the implementation of the HeaderEventSubscription that receives each block and checks
// tx confirmation
func (t *KlaytnTransactionConfirmer) ReceiveBlock(block NodeBlock) error {
	if block.BlockInterface == nil {
		log.Info().Msg("Received nil block")
		return nil
	}
	confirmationLog := log.Debug().Str("Network", t.eth.Network.ID()).
		Str("Block Hash", block.GetHash().Hex()).
		Str("Block Number", block.Number().String()).
		Str("Tx Hash", t.tx.Hash().String()).
		Uint64("Nonce", t.tx.Nonce()).
		Int("Minimum Confirmations", t.minConfirmations)
	isConfirmed, err := t.eth.isTxConfirmed(t.tx.Hash())
	if err != nil {
		return err
	} else if isConfirmed {
		t.confirmations++
	}
	if t.confirmations == t.minConfirmations {
		confirmationLog.Int("Current Confirmations", t.confirmations).
			Msg("Transaction confirmations met")
		t.doneChan <- struct{}{}
	} else if t.confirmations <= t.minConfirmations {
		confirmationLog.Int("Current Confirmations", t.confirmations).
			Msg("Waiting on minimum confirmations")
	}
	return nil
}

// Wait is a blocking function that waits until the transaction is complete
func (t *KlaytnTransactionConfirmer) Wait() error {
	for {
		select {
		case <-t.doneChan:
			t.cancel()
			return nil
		case <-t.context.Done():
			return fmt.Errorf("timeout waiting for transaction to confirm: %s", t.tx.Hash().String())
		}
	}
}

// KlaytnInstantConfirmations is a no-op confirmer as all transactions are instantly mined so no confs are needed
type KlaytnInstantConfirmations struct{}

// ReceiveBlock is a no-op
func (i *KlaytnInstantConfirmations) ReceiveBlock(block NodeBlock) error {
	return nil
}

// Wait is a no-op
func (i *KlaytnInstantConfirmations) Wait() error {
	return nil
}
