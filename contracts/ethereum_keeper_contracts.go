package contracts

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"time"

	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/rs/zerolog/log"
	"github.com/smartcontractkit/chainlink-testing-framework/blockchain"
	"github.com/smartcontractkit/chainlink-testing-framework/contracts/ethereum"
	"github.com/smartcontractkit/chainlink-testing-framework/testreporters"
)

type UpkeepRegistrar interface {
	Address() string
	SetRegistrarConfig(
		autoRegister bool,
		windowSizeBlocks uint32,
		allowedPerWindow uint16,
		registryAddr string,
		minLinkJuels *big.Int,
	) error
	EncodeRegisterRequest(
		name string,
		email []byte,
		upkeepAddr string,
		gasLimit uint32,
		adminAddr string,
		checkData []byte,
		amount *big.Int,
		source uint8,
	) ([]byte, error)
	Fund(ethAmount *big.Float) error
}

type KeeperRegistry interface {
	Address() string
	Fund(ethAmount *big.Float) error
	SetConfig(config KeeperRegistrySettings) error
	SetRegistrar(registrarAddr string) error
	AddUpkeepFunds(id *big.Int, amount *big.Int) error
	GetUpkeepInfo(ctx context.Context, id *big.Int) (*UpkeepInfo, error)
	GetKeeperInfo(ctx context.Context, keeperAddr string) (*KeeperInfo, error)
	SetKeepers(keepers []string, payees []string) error
	GetKeeperList(ctx context.Context) ([]string, error)
	RegisterUpkeep(target string, gasLimit uint32, admin string, checkData []byte) error
	CancelUpkeep(id *big.Int) error
	SetUpkeepGasLimit(id *big.Int, gas uint32) error
	ParseUpkeepIdFromRegisteredLog(log *types.Log) (*big.Int, error)
}

type KeeperConsumer interface {
	Address() string
	Fund(ethAmount *big.Float) error
	Counter(ctx context.Context) (*big.Int, error)
}

type UpkeepCounter interface {
	Address() string
	Fund(ethAmount *big.Float) error
	Counter(ctx context.Context) (*big.Int, error)
	SetSpread(testRange *big.Int, interval *big.Int) error
}

type UpkeepPerformCounterRestrictive interface {
	Address() string
	Fund(ethAmount *big.Float) error
	Counter(ctx context.Context) (*big.Int, error)
	SetSpread(testRange *big.Int, interval *big.Int) error
}

// KeeperConsumerPerformance is a keeper consumer contract that is more complicated than the typical consumer,
// it's intended to only be used for performance tests.
type KeeperConsumerPerformance interface {
	Address() string
	Fund(ethAmount *big.Float) error
	CheckEligible(ctx context.Context) (bool, error)
	GetUpkeepCount(ctx context.Context) (*big.Int, error)
	SetCheckGasToBurn(ctx context.Context, gas *big.Int) error
	SetPerformGasToBurn(ctx context.Context, gas *big.Int) error
}

// KeeperRegistryOpts opts to deploy keeper registry version
type KeeperRegistryOpts struct {
	RegistryVersion ethereum.KeeperRegistryVersion
	LinkAddr        string
	ETHFeedAddr     string
	GasFeedAddr     string
	TranscoderAddr  string
	RegistrarAddr   string
	Settings        KeeperRegistrySettings
}

// KeeperRegistrySettings represents the settins to fine tune keeper registry
type KeeperRegistrySettings struct {
	PaymentPremiumPPB    uint32   // payment premium rate oracles receive on top of being reimbursed for gas, measured in parts per billion
	FlatFeeMicroLINK     uint32   // flat fee charged for each upkeep
	BlockCountPerTurn    *big.Int // number of blocks each oracle has during their turn to perform upkeep before it will be the next keeper's turn to submit
	CheckGasLimit        uint32   // gas limit when checking for upkeep
	StalenessSeconds     *big.Int // number of seconds that is allowed for feed data to be stale before switching to the fallback pricing
	GasCeilingMultiplier uint16   // multiplier to apply to the fast gas feed price when calculating the payment ceiling for keepers
	MinUpkeepSpend       *big.Int // minimum spend required by an upkeep before they can withdraw funds
	MaxPerformGas        uint32   // max gas allowed for an upkeep within perform
	FallbackGasPrice     *big.Int // gas price used if the gas price feed is stale
	FallbackLinkPrice    *big.Int // LINK price used if the LINK price feed is stale
}

// KeeperRegistrarSettings represents settings for registrar contract
type KeeperRegistrarSettings struct {
	AutoRegister     bool
	WindowSizeBlocks uint32
	AllowedPerWindow uint16
	RegistryAddr     string
	MinLinkJuels     *big.Int
}

// KeeperInfo keeper status and balance info
type KeeperInfo struct {
	Payee   string
	Active  bool
	Balance *big.Int
}

// UpkeepInfo keeper target info
type UpkeepInfo struct {
	Target              string
	ExecuteGas          uint32
	CheckData           []byte
	Balance             *big.Int
	LastKeeper          string
	Admin               string
	MaxValidBlocknumber uint64
}

// EthereumKeeperRegistry represents keeper registry contract
type EthereumKeeperRegistry struct {
	client      blockchain.EVMClient
	version     ethereum.KeeperRegistryVersion
	registry1_1 *ethereum.KeeperRegistry11
	registry1_2 *ethereum.KeeperRegistry
	address     *common.Address
}

func (v *EthereumKeeperRegistry) Address() string {
	return v.address.Hex()
}

func (v *EthereumKeeperRegistry) Fund(ethAmount *big.Float) error {
	return v.client.Fund(v.address.Hex(), ethAmount)
}

func (v *EthereumKeeperRegistry) SetConfig(config KeeperRegistrySettings) error {
	txOpts, err := v.client.TransactionOpts(v.client.GetDefaultWallet())
	if err != nil {
		return err
	}
	callOpts := bind.CallOpts{
		From:    common.HexToAddress(v.client.GetDefaultWallet().Address()),
		Context: nil,
	}
	switch v.version {
	case ethereum.RegistryVersion_1_0, ethereum.RegistryVersion_1_1:
		tx, err := v.registry1_1.SetConfig(
			txOpts,
			config.PaymentPremiumPPB,
			config.FlatFeeMicroLINK,
			config.BlockCountPerTurn,
			config.CheckGasLimit,
			config.StalenessSeconds,
			config.GasCeilingMultiplier,
			config.FallbackGasPrice,
			config.FallbackLinkPrice,
		)
		if err != nil {
			return err
		}
		return v.client.ProcessTransaction(tx)
	case ethereum.RegistryVersion_1_2:
		state, err := v.registry1_2.GetState(&callOpts)
		if err != nil {
			return err
		}

		tx, err := v.registry1_2.SetConfig(txOpts, ethereum.Config{
			PaymentPremiumPPB:    config.PaymentPremiumPPB,
			FlatFeeMicroLink:     config.FlatFeeMicroLINK,
			BlockCountPerTurn:    config.BlockCountPerTurn,
			CheckGasLimit:        config.CheckGasLimit,
			StalenessSeconds:     config.StalenessSeconds,
			GasCeilingMultiplier: config.GasCeilingMultiplier,
			MinUpkeepSpend:       config.MinUpkeepSpend,
			MaxPerformGas:        config.MaxPerformGas,
			FallbackGasPrice:     config.FallbackGasPrice,
			FallbackLinkPrice:    config.FallbackLinkPrice,
			// Keep the transcoder and registrar same. They have separate setters
			Transcoder: state.Config.Transcoder,
			Registrar:  state.Config.Registrar,
		})
		if err != nil {
			return err
		}
		return v.client.ProcessTransaction(tx)
	}

	return fmt.Errorf("keeper registry version %d is not supported", v.version)
}

func (v *EthereumKeeperRegistry) SetRegistrar(registrarAddr string) error {
	txOpts, err := v.client.TransactionOpts(v.client.GetDefaultWallet())
	if err != nil {
		return err
	}
	callOpts := bind.CallOpts{
		From:    common.HexToAddress(v.client.GetDefaultWallet().Address()),
		Context: nil,
	}

	switch v.version {
	case ethereum.RegistryVersion_1_0, ethereum.RegistryVersion_1_1:
		tx, err := v.registry1_1.SetRegistrar(txOpts, common.HexToAddress(registrarAddr))
		if err != nil {
			return err
		}
		return v.client.ProcessTransaction(tx)
	case ethereum.RegistryVersion_1_2:
		state, err := v.registry1_2.GetState(&callOpts)
		if err != nil {
			return err
		}
		newConfig := state.Config
		newConfig.Registrar = common.HexToAddress(registrarAddr)
		tx, err := v.registry1_2.SetConfig(txOpts, newConfig)
		if err != nil {
			return err
		}
		return v.client.ProcessTransaction(tx)
	}

	return fmt.Errorf("keeper registry version %d is not supported", v.version)
}

// AddUpkeepFunds adds link for particular upkeep id
func (v *EthereumKeeperRegistry) AddUpkeepFunds(id *big.Int, amount *big.Int) error {
	opts, err := v.client.TransactionOpts(v.client.GetDefaultWallet())
	if err != nil {
		return err
	}
	var tx *types.Transaction

	switch v.version {
	case ethereum.RegistryVersion_1_0, ethereum.RegistryVersion_1_1:
		tx, err = v.registry1_1.AddFunds(opts, id, amount)
	case ethereum.RegistryVersion_1_2:
		tx, err = v.registry1_2.AddFunds(opts, id, amount)
	}

	if err != nil {
		return err
	}
	return v.client.ProcessTransaction(tx)
}

// GetUpkeepInfo gets upkeep info
func (v *EthereumKeeperRegistry) GetUpkeepInfo(ctx context.Context, id *big.Int) (*UpkeepInfo, error) {
	opts := &bind.CallOpts{
		From:    common.HexToAddress(v.client.GetDefaultWallet().Address()),
		Context: ctx,
	}

	switch v.version {
	case ethereum.RegistryVersion_1_0, ethereum.RegistryVersion_1_1:
		uk, err := v.registry1_1.GetUpkeep(opts, id)
		if err != nil {
			return nil, err
		}
		return &UpkeepInfo{
			Target:              uk.Target.Hex(),
			ExecuteGas:          uk.ExecuteGas,
			CheckData:           uk.CheckData,
			Balance:             uk.Balance,
			LastKeeper:          uk.LastKeeper.Hex(),
			Admin:               uk.Admin.Hex(),
			MaxValidBlocknumber: uk.MaxValidBlocknumber,
		}, nil
	case ethereum.RegistryVersion_1_2:
		uk, err := v.registry1_2.GetUpkeep(opts, id)
		if err != nil {
			return nil, err
		}
		return &UpkeepInfo{
			Target:              uk.Target.Hex(),
			ExecuteGas:          uk.ExecuteGas,
			CheckData:           uk.CheckData,
			Balance:             uk.Balance,
			LastKeeper:          uk.LastKeeper.Hex(),
			Admin:               uk.Admin.Hex(),
			MaxValidBlocknumber: uk.MaxValidBlocknumber,
		}, nil
	}

	return nil, fmt.Errorf("keeper registry version %d is not supported", v.version)
}

func (v *EthereumKeeperRegistry) GetKeeperInfo(ctx context.Context, keeperAddr string) (*KeeperInfo, error) {
	opts := &bind.CallOpts{
		From:    common.HexToAddress(v.client.GetDefaultWallet().Address()),
		Context: ctx,
	}
	var info struct {
		Payee   common.Address
		Active  bool
		Balance *big.Int
	}
	var err error

	switch v.version {
	case ethereum.RegistryVersion_1_0, ethereum.RegistryVersion_1_1:
		info, err = v.registry1_1.GetKeeperInfo(opts, common.HexToAddress(keeperAddr))
	case ethereum.RegistryVersion_1_2:
		info, err = v.registry1_2.GetKeeperInfo(opts, common.HexToAddress(keeperAddr))
	}

	if err != nil {
		return nil, err
	}
	return &KeeperInfo{
		Payee:   info.Payee.Hex(),
		Active:  info.Active,
		Balance: info.Balance,
	}, nil
}

func (v *EthereumKeeperRegistry) SetKeepers(keepers []string, payees []string) error {
	opts, err := v.client.TransactionOpts(v.client.GetDefaultWallet())
	if err != nil {
		return err
	}
	keepersAddresses := make([]common.Address, 0)
	for _, k := range keepers {
		keepersAddresses = append(keepersAddresses, common.HexToAddress(k))
	}
	payeesAddresses := make([]common.Address, 0)
	for _, p := range payees {
		payeesAddresses = append(payeesAddresses, common.HexToAddress(p))
	}
	var tx *types.Transaction

	switch v.version {
	case ethereum.RegistryVersion_1_0, ethereum.RegistryVersion_1_1:
		tx, err = v.registry1_1.SetKeepers(opts, keepersAddresses, payeesAddresses)
	case ethereum.RegistryVersion_1_2:
		tx, err = v.registry1_2.SetKeepers(opts, keepersAddresses, payeesAddresses)
	}

	if err != nil {
		return err
	}
	return v.client.ProcessTransaction(tx)
}

// RegisterUpkeep registers contract to perform upkeep
func (v *EthereumKeeperRegistry) RegisterUpkeep(target string, gasLimit uint32, admin string, checkData []byte) error {
	opts, err := v.client.TransactionOpts(v.client.GetDefaultWallet())
	if err != nil {
		return err
	}
	var tx *types.Transaction

	switch v.version {
	case ethereum.RegistryVersion_1_0, ethereum.RegistryVersion_1_1:
		tx, err = v.registry1_1.RegisterUpkeep(
			opts,
			common.HexToAddress(target),
			gasLimit,
			common.HexToAddress(admin),
			checkData,
		)
	case ethereum.RegistryVersion_1_2:
		tx, err = v.registry1_2.RegisterUpkeep(
			opts,
			common.HexToAddress(target),
			gasLimit,
			common.HexToAddress(admin),
			checkData,
		)
	}

	if err != nil {
		return err
	}
	return v.client.ProcessTransaction(tx)
}

// CancelUpkeep cancels the given upkeep ID
func (v *EthereumKeeperRegistry) CancelUpkeep(id *big.Int) error {
	opts, err := v.client.TransactionOpts(v.client.GetDefaultWallet())
	if err != nil {
		return err
	}
	var tx *types.Transaction

	switch v.version {
	case ethereum.RegistryVersion_1_0, ethereum.RegistryVersion_1_1:
		tx, err = v.registry1_1.CancelUpkeep(opts, id)
		if err != nil {
			return err
		}
	case ethereum.RegistryVersion_1_2:
		tx, err = v.registry1_2.CancelUpkeep(opts, id)
		if err != nil {
			return err
		}
	}

	log.Info().
		Str("Upkeep ID", strconv.FormatInt(id.Int64(), 10)).
		Str("From", v.client.GetDefaultWallet().Address()).
		Str("TX Hash", tx.Hash().String()).
		Msg("Cancel Upkeep tx")
	return v.client.ProcessTransaction(tx)
}

// SetUpkeepGasLimit sets the perform gas limit for a given upkeep ID
func (v *EthereumKeeperRegistry) SetUpkeepGasLimit(id *big.Int, gas uint32) error {
	opts, err := v.client.TransactionOpts(v.client.GetDefaultWallet())
	if err != nil {
		return err
	}
	var tx *types.Transaction

	switch v.version {
	case ethereum.RegistryVersion_1_2:
		tx, err = v.registry1_2.SetUpkeepGasLimit(opts, id, gas)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("keeper registry version %d is not supported for SetUpkeepGasLimit", v.version)
	}
	return v.client.ProcessTransaction(tx)
}

// GetKeeperList get list of all registered keeper addresses
func (v *EthereumKeeperRegistry) GetKeeperList(ctx context.Context) ([]string, error) {
	opts := &bind.CallOpts{
		From:    common.HexToAddress(v.client.GetDefaultWallet().Address()),
		Context: ctx,
	}
	var list []common.Address
	var err error

	switch v.version {
	case ethereum.RegistryVersion_1_0, ethereum.RegistryVersion_1_1:
		list, err = v.registry1_1.GetKeeperList(opts)
	case ethereum.RegistryVersion_1_2:
		state, err := v.registry1_2.GetState(opts)
		if err != nil {
			return []string{}, err
		}
		list = state.Keepers
	}

	if err != nil {
		return []string{}, err
	}
	addrs := make([]string, 0)
	for _, ca := range list {
		addrs = append(addrs, ca.Hex())
	}
	return addrs, nil
}

// Parses the upkeep ID from an 'UpkeepRegistered' log, returns error on any other log
func (v *EthereumKeeperRegistry) ParseUpkeepIdFromRegisteredLog(log *types.Log) (*big.Int, error) {
	switch v.version {
	case ethereum.RegistryVersion_1_0, ethereum.RegistryVersion_1_1:
		parsedLog, err := v.registry1_1.ParseUpkeepRegistered(*log)
		if err != nil {
			return nil, err
		}
		return parsedLog.Id, nil
	case ethereum.RegistryVersion_1_2:
		parsedLog, err := v.registry1_2.ParseUpkeepRegistered(*log)
		if err != nil {
			return nil, err
		}
		return parsedLog.Id, nil
	}
	return nil, fmt.Errorf("keeper registry version %d is not supported", v.version)
}

// KeeperConsumerRoundConfirmer is a header subscription that awaits for a round of upkeeps
type KeeperConsumerRoundConfirmer struct {
	instance     KeeperConsumer
	upkeepsValue int
	doneChan     chan struct{}
	context      context.Context
	cancel       context.CancelFunc
}

// NewKeeperConsumerRoundConfirmer provides a new instance of a KeeperConsumerRoundConfirmer
func NewKeeperConsumerRoundConfirmer(
	contract KeeperConsumer,
	counterValue int,
	timeout time.Duration,
) *KeeperConsumerRoundConfirmer {
	ctx, ctxCancel := context.WithTimeout(context.Background(), timeout)
	return &KeeperConsumerRoundConfirmer{
		instance:     contract,
		upkeepsValue: counterValue,
		doneChan:     make(chan struct{}),
		context:      ctx,
		cancel:       ctxCancel,
	}
}

// ReceiveBlock will query the latest Keeper round and check to see whether the round has confirmed
func (o *KeeperConsumerRoundConfirmer) ReceiveBlock(_ blockchain.NodeBlock) error {
	upkeeps, err := o.instance.Counter(context.Background())
	if err != nil {
		return err
	}
	l := log.Info().
		Str("Contract Address", o.instance.Address()).
		Int64("Upkeeps", upkeeps.Int64()).
		Int("Required upkeeps", o.upkeepsValue)
	if upkeeps.Int64() == int64(o.upkeepsValue) {
		l.Msg("Upkeep completed")
		o.doneChan <- struct{}{}
	} else {
		l.Msg("Waiting for upkeep round")
	}
	return nil
}

// Wait is a blocking function that will wait until the round has confirmed, and timeout if the deadline has passed
func (o *KeeperConsumerRoundConfirmer) Wait() error {
	for {
		select {
		case <-o.doneChan:
			o.cancel()
			return nil
		case <-o.context.Done():
			return fmt.Errorf("timeout waiting for upkeeps to confirm: %d", o.upkeepsValue)
		}
	}
}

// KeeperConsumerPerformanceRoundConfirmer is a header subscription that awaits for a round of upkeeps
type KeeperConsumerPerformanceRoundConfirmer struct {
	instance KeeperConsumerPerformance
	doneChan chan bool
	context  context.Context
	cancel   context.CancelFunc

	blockCadence                int64   // How many blocks before an upkeep should happen
	blockRange                  int64   // How many blocks to watch upkeeps for
	blocksSinceSubscription     int64   // How many blocks have passed since subscribing
	expectedUpkeepCount         int64   // The count of upkeeps expected next iteration
	blocksSinceSuccessfulUpkeep int64   // How many blocks have come in since the last successful upkeep
	allMissedUpkeeps            []int64 // Tracks the amount of blocks missed in each missed upkeep
	totalSuccessfulUpkeeps      int64

	metricsReporter *testreporters.KeeperBlockTimeTestReporter // Testreporter to track results
}

// NewKeeperConsumerPerformanceRoundConfirmer provides a new instance of a KeeperConsumerPerformanceRoundConfirmer
// Used to track and log performance test results for keepers
func NewKeeperConsumerPerformanceRoundConfirmer(
	contract KeeperConsumerPerformance,
	expectedBlockCadence int64, // Expected to upkeep every 5/10/20 blocks, for example
	blockRange int64,
	metricsReporter *testreporters.KeeperBlockTimeTestReporter,
) *KeeperConsumerPerformanceRoundConfirmer {
	ctx, cancelFunc := context.WithCancel(context.Background())
	return &KeeperConsumerPerformanceRoundConfirmer{
		instance:                    contract,
		doneChan:                    make(chan bool),
		context:                     ctx,
		cancel:                      cancelFunc,
		blockCadence:                expectedBlockCadence,
		blockRange:                  blockRange,
		blocksSinceSubscription:     0,
		blocksSinceSuccessfulUpkeep: 0,
		expectedUpkeepCount:         1,
		allMissedUpkeeps:            []int64{},
		totalSuccessfulUpkeeps:      0,
		metricsReporter:             metricsReporter,
	}
}

// ReceiveBlock will query the latest Keeper round and check to see whether the round has confirmed
func (o *KeeperConsumerPerformanceRoundConfirmer) ReceiveBlock(receivedBlock blockchain.NodeBlock) error {
	// Increment block counters
	o.blocksSinceSubscription++
	o.blocksSinceSuccessfulUpkeep++
	upkeepCount, err := o.instance.GetUpkeepCount(context.Background())
	if err != nil {
		return err
	}

	isEligible, err := o.instance.CheckEligible(context.Background())
	if err != nil {
		return err
	}
	if isEligible {
		log.Trace().
			Str("Contract Address", o.instance.Address()).
			Int64("Upkeeps Performed", upkeepCount.Int64()).
			Msg("Upkeep Now Eligible")
	}
	if upkeepCount.Int64() >= o.expectedUpkeepCount { // Upkeep was successful
		if o.blocksSinceSuccessfulUpkeep < o.blockCadence { // If there's an early upkeep, that's weird
			log.Error().
				Str("Contract Address", o.instance.Address()).
				Int64("Upkeeps Performed", upkeepCount.Int64()).
				Int64("Expected Cadence", o.blockCadence).
				Int64("Actual Cadence", o.blocksSinceSuccessfulUpkeep).
				Err(errors.New("Found an early Upkeep"))
			return fmt.Errorf("Found an early Upkeep on contract %s", o.instance.Address())
		} else if o.blocksSinceSuccessfulUpkeep == o.blockCadence { // Perfectly timed upkeep
			log.Info().
				Str("Contract Address", o.instance.Address()).
				Int64("Upkeeps Performed", upkeepCount.Int64()).
				Int64("Expected Cadence", o.blockCadence).
				Int64("Actual Cadence", o.blocksSinceSuccessfulUpkeep).
				Msg("Successful Upkeep on Expected Cadence")
			o.totalSuccessfulUpkeeps++
		} else { // Late upkeep
			log.Warn().
				Str("Contract Address", o.instance.Address()).
				Int64("Upkeeps Performed", upkeepCount.Int64()).
				Int64("Expected Cadence", o.blockCadence).
				Int64("Actual Cadence", o.blocksSinceSuccessfulUpkeep).
				Msg("Upkeep Completed Late")
			o.allMissedUpkeeps = append(o.allMissedUpkeeps, o.blocksSinceSuccessfulUpkeep-o.blockCadence)
		}
		// Update upkeep tracking values
		o.blocksSinceSuccessfulUpkeep = 0
		o.expectedUpkeepCount++
	}

	if o.blocksSinceSubscription > o.blockRange {
		if o.blocksSinceSuccessfulUpkeep > o.blockCadence {
			log.Warn().
				Str("Contract Address", o.instance.Address()).
				Int64("Upkeeps Performed", upkeepCount.Int64()).
				Int64("Expected Cadence", o.blockCadence).
				Int64("Expected Upkeep Count", o.expectedUpkeepCount).
				Int64("Blocks Waiting", o.blocksSinceSuccessfulUpkeep).
				Int64("Total Blocks Watched", o.blocksSinceSubscription).
				Msg("Finished Watching for Upkeeps While Waiting on a Late Upkeep")
			o.allMissedUpkeeps = append(o.allMissedUpkeeps, o.blocksSinceSuccessfulUpkeep-o.blockCadence)
		} else {
			log.Info().
				Str("Contract Address", o.instance.Address()).
				Int64("Upkeeps Performed", upkeepCount.Int64()).
				Int64("Total Blocks Watched", o.blocksSinceSubscription).
				Msg("Finished Watching for Upkeeps")
		}
		o.doneChan <- true
		return nil
	}
	return nil
}

// Wait is a blocking function that will wait until the round has confirmed, and timeout if the deadline has passed
func (o *KeeperConsumerPerformanceRoundConfirmer) Wait() error {
	for {
		select {
		case <-o.doneChan:
			o.cancel()
			o.logDetails()
			return nil
		case <-o.context.Done():
			return fmt.Errorf("timeout waiting for expected upkeep count to confirm: %d", o.expectedUpkeepCount)
		}
	}
}

func (o *KeeperConsumerPerformanceRoundConfirmer) logDetails() {
	report := testreporters.KeeperBlockTimeTestReport{
		ContractAddress:        o.instance.Address(),
		TotalExpectedUpkeeps:   o.blockRange / o.blockCadence,
		TotalSuccessfulUpkeeps: o.totalSuccessfulUpkeeps,
		AllMissedUpkeeps:       o.allMissedUpkeeps,
	}
	o.metricsReporter.ReportMutex.Lock()
	o.metricsReporter.Reports = append(o.metricsReporter.Reports, report)
	defer o.metricsReporter.ReportMutex.Unlock()
}

// EthereumUpkeepCounter represents keeper consumer (upkeep) counter contract
type EthereumUpkeepCounter struct {
	client   blockchain.EVMClient
	consumer *ethereum.UpkeepCounter
	address  *common.Address
}

func (v *EthereumUpkeepCounter) Address() string {
	return v.address.Hex()
}

func (v *EthereumUpkeepCounter) Fund(ethAmount *big.Float) error {
	return v.client.Fund(v.address.Hex(), ethAmount)
}
func (v *EthereumUpkeepCounter) Counter(ctx context.Context) (*big.Int, error) {
	opts := &bind.CallOpts{
		From:    common.HexToAddress(v.client.GetDefaultWallet().Address()),
		Context: ctx,
	}
	cnt, err := v.consumer.Counter(opts)
	if err != nil {
		return nil, err
	}
	return cnt, nil
}

func (v *EthereumUpkeepCounter) SetSpread(testRange *big.Int, interval *big.Int) error {
	opts, err := v.client.TransactionOpts(v.client.GetDefaultWallet())
	if err != nil {
		return err
	}
	tx, err := v.consumer.SetSpread(opts, testRange, interval)
	if err != nil {
		return err
	}
	return v.client.ProcessTransaction(tx)
}

// EthereumUpkeepPerformCounterRestrictive represents keeper consumer (upkeep) counter contract
type EthereumUpkeepPerformCounterRestrictive struct {
	client   blockchain.EVMClient
	consumer *ethereum.UpkeepPerformCounterRestrictive
	address  *common.Address
}

func (v *EthereumUpkeepPerformCounterRestrictive) Address() string {
	return v.address.Hex()
}

func (v *EthereumUpkeepPerformCounterRestrictive) Fund(ethAmount *big.Float) error {
	return v.client.Fund(v.address.Hex(), ethAmount)
}
func (v *EthereumUpkeepPerformCounterRestrictive) Counter(ctx context.Context) (*big.Int, error) {
	opts := &bind.CallOpts{
		From:    common.HexToAddress(v.client.GetDefaultWallet().Address()),
		Context: ctx,
	}
	count, err := v.consumer.GetCountPerforms(opts)
	return count, err
}

func (v *EthereumUpkeepPerformCounterRestrictive) SetSpread(testRange *big.Int, interval *big.Int) error {
	opts, err := v.client.TransactionOpts(v.client.GetDefaultWallet())
	if err != nil {
		return err
	}
	tx, err := v.consumer.SetSpread(opts, testRange, interval)
	if err != nil {
		return err
	}
	return v.client.ProcessTransaction(tx)
}

// EthereumKeeperConsumer represents keeper consumer (upkeep) contract
type EthereumKeeperConsumer struct {
	client   blockchain.EVMClient
	consumer *ethereum.KeeperConsumer
	address  *common.Address
}

func (v *EthereumKeeperConsumer) Address() string {
	return v.address.Hex()
}

func (v *EthereumKeeperConsumer) Fund(ethAmount *big.Float) error {
	return v.client.Fund(v.address.Hex(), ethAmount)
}

func (v *EthereumKeeperConsumer) Counter(ctx context.Context) (*big.Int, error) {
	opts := &bind.CallOpts{
		From:    common.HexToAddress(v.client.GetDefaultWallet().Address()),
		Context: ctx,
	}
	cnt, err := v.consumer.Counter(opts)
	if err != nil {
		return nil, err
	}
	return cnt, nil
}

// EthereumKeeperConsumerPerformance represents a more complicated keeper consumer contract, one intended only for
// performance tests.
type EthereumKeeperConsumerPerformance struct {
	client   blockchain.EVMClient
	consumer *ethereum.KeeperConsumerPerformance
	address  *common.Address
}

func (v *EthereumKeeperConsumerPerformance) Address() string {
	return v.address.Hex()
}

func (v *EthereumKeeperConsumerPerformance) Fund(ethAmount *big.Float) error {
	return v.client.Fund(v.address.Hex(), ethAmount)
}

func (v *EthereumKeeperConsumerPerformance) CheckEligible(ctx context.Context) (bool, error) {
	opts := &bind.CallOpts{
		From:    common.HexToAddress(v.client.GetDefaultWallet().Address()),
		Context: ctx,
	}
	eligible, err := v.consumer.CheckEligible(opts)
	return eligible, err
}

func (v *EthereumKeeperConsumerPerformance) GetUpkeepCount(ctx context.Context) (*big.Int, error) {
	opts := &bind.CallOpts{
		From:    common.HexToAddress(v.client.GetDefaultWallet().Address()),
		Context: ctx,
	}
	eligible, err := v.consumer.GetCountPerforms(opts)
	return eligible, err
}

func (v *EthereumKeeperConsumerPerformance) SetCheckGasToBurn(ctx context.Context, gas *big.Int) error {
	opts, err := v.client.TransactionOpts(v.client.GetDefaultWallet())
	if err != nil {
		return err
	}
	tx, err := v.consumer.SetCheckGasToBurn(opts, gas)
	if err != nil {
		return err
	}
	return v.client.ProcessTransaction(tx)
}

func (v *EthereumKeeperConsumerPerformance) SetPerformGasToBurn(ctx context.Context, gas *big.Int) error {
	opts, err := v.client.TransactionOpts(v.client.GetDefaultWallet())
	if err != nil {
		return err
	}
	tx, err := v.consumer.SetPerformGasToBurn(opts, gas)
	if err != nil {
		return err
	}
	return v.client.ProcessTransaction(tx)
}

// EthereumUpkeepRegistrationRequests keeper contract to register upkeeps
type EthereumUpkeepRegistrationRequests struct {
	client    blockchain.EVMClient
	registrar *ethereum.UpkeepRegistrationRequests
	address   *common.Address
}

func (v *EthereumUpkeepRegistrationRequests) Address() string {
	return v.address.Hex()
}

// SetRegistrarConfig sets registrar config, allowing auto register or pending requests for manual registration
func (v *EthereumUpkeepRegistrationRequests) SetRegistrarConfig(
	autoRegister bool,
	windowSizeBlocks uint32,
	allowedPerWindow uint16,
	registryAddr string,
	minLinkJuels *big.Int,
) error {
	opts, err := v.client.TransactionOpts(v.client.GetDefaultWallet())
	if err != nil {
		return err
	}
	tx, err := v.registrar.SetRegistrationConfig(opts, autoRegister, windowSizeBlocks, allowedPerWindow, common.HexToAddress(registryAddr), minLinkJuels)
	if err != nil {
		return err
	}
	return v.client.ProcessTransaction(tx)
}

func (v *EthereumUpkeepRegistrationRequests) Fund(ethAmount *big.Float) error {
	return v.client.Fund(v.address.Hex(), ethAmount)
}

// EncodeRegisterRequest encodes register request to call it through link token TransferAndCall
func (v *EthereumUpkeepRegistrationRequests) EncodeRegisterRequest(
	name string,
	email []byte,
	upkeepAddr string,
	gasLimit uint32,
	adminAddr string,
	checkData []byte,
	amount *big.Int,
	source uint8,
) ([]byte, error) {
	registryABI, err := abi.JSON(strings.NewReader(ethereum.UpkeepRegistrationRequestsABI))
	if err != nil {
		return nil, err
	}
	req, err := registryABI.Pack(
		"register",
		name,
		email,
		common.HexToAddress(upkeepAddr),
		gasLimit,
		common.HexToAddress(adminAddr),
		checkData,
		amount,
		source,
	)
	if err != nil {
		return nil, err
	}
	return req, nil
}
