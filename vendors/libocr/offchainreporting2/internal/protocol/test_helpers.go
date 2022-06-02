package protocol

import (
	"github.com/smartcontractkit/chainlink-testing-framework/vendors/libocr/commontypes"
	"github.com/smartcontractkit/chainlink-testing-framework/vendors/libocr/offchainreporting2/types"
)

// Used only for testing
type XXXUnknownMessageType struct{}

// Conform to protocol.Message interface
func (XXXUnknownMessageType) CheckSize(types.ReportingPluginLimits) bool { return true }
func (XXXUnknownMessageType) process(*oracleState, commontypes.OracleID) {}
