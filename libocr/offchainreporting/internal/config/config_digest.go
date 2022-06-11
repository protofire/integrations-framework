package config

import (
	"fmt"
	"strings"

	"github.com/celo-org/celo-blockchain/accounts/abi"
	"github.com/celo-org/celo-blockchain/common"
	"github.com/celo-org/celo-blockchain/crypto"

	"github.com/smartcontractkit/chainlink-testing-framework/libocr/gethwrappers/exposedoffchainaggregator"

	"github.com/smartcontractkit/chainlink-testing-framework/libocr/offchainreporting/types"
)

func makeConfigDigestArgs() abi.Arguments {
	_abi, err := abi.JSON(strings.NewReader(
		exposedoffchainaggregator.ExposedOffchainAggregatorABI))
	if err != nil {
		var _err any
		_err = fmt.Sprintf("could not parse aggregator ABI: %s", err.Error())
		// assertion
		panic(_err)
	}
	return _abi.Methods["exposedConfigDigestFromConfigData"].Inputs
}

var configDigestArgs = makeConfigDigestArgs()

func ConfigDigest(
	contractAddress common.Address,
	configCount uint64,
	oracles []common.Address,
	transmitters []common.Address,
	threshold uint8,
	encodedConfigVersion uint64,
	config []byte,
) types.ConfigDigest {
	msg, err := configDigestArgs.Pack(
		contractAddress,
		configCount,
		oracles,
		transmitters,
		threshold,
		encodedConfigVersion,
		config,
	)
	if err != nil {
		// assertion
		var _err any

		_err = err
		panic(_err)
	}
	rawHash := crypto.Keccak256(msg)
	configDigest := types.ConfigDigest{}
	if n := copy(configDigest[:], rawHash); n != len(configDigest) {
		var _err any
		_err = "copy too little data"
		// assertion
		panic(_err)
	}
	return configDigest
}
