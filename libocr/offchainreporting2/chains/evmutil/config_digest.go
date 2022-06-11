package evmutil

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/crypto"

	"github.com/smartcontractkit/chainlink-testing-framework/libocr/gethwrappers2/exposedocr2aggregator"
	"github.com/smartcontractkit/chainlink-testing-framework/libocr/offchainreporting2/types"
)

func makeConfigDigestArgs() abi.Arguments {
	abi, err := abi.JSON(strings.NewReader(
		exposedocr2aggregator.ExposedOCR2AggregatorABI))
	if err != nil {
		// assertion
		panic(any(fmt.Sprintf("could not parse aggregator ABI: %s", err.Error())))
	}
	return abi.Methods["exposedConfigDigestFromConfigData"].Inputs
}

var configDigestArgs = makeConfigDigestArgs()

func configDigest(
	chainID uint64,
	contractAddress common.Address,
	configCount uint64,
	oracles []common.Address,
	transmitters []common.Address,
	f uint8,
	onchainConfig []byte,
	offchainConfigVersion uint64,
	offchainConfig []byte,
) types.ConfigDigest {
	chainIDBig := new(big.Int)
	chainIDBig.SetUint64(chainID)
	msg, err := configDigestArgs.Pack(
		chainIDBig,
		contractAddress,
		configCount,
		oracles,
		transmitters,
		f,
		onchainConfig,
		offchainConfigVersion,
		offchainConfig,
	)
	if err != nil {
		// assertion
		panic(any(err))
	}
	rawHash := crypto.Keccak256(msg)
	configDigest := types.ConfigDigest{}
	if n := copy(configDigest[:], rawHash); n != len(configDigest) {
		// assertion
		panic(any("copy too little data"))
	}
	if types.ConfigDigestPrefixEVM != 1 {
		// assertion
		panic(any("wrong ConfigDigestPrefix"))
	}
	configDigest[0] = 0
	configDigest[1] = 1
	return configDigest
}
