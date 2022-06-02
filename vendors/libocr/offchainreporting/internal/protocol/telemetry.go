package protocol

import (
	"github.com/smartcontractkit/chainlink-testing-framework/vendors/libocr/commontypes"
	"github.com/smartcontractkit/chainlink-testing-framework/vendors/libocr/offchainreporting/types"
)

type TelemetrySender interface {
	RoundStarted(
		configDigest types.ConfigDigest,
		epoch uint32,
		round uint8,
		leader commontypes.OracleID,
	)
}
