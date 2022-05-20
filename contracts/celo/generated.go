package celo

import (
	"github.com/celo-org/celo-blockchain/common"
)

// AbigenLog is an interface for abigen generated log topics
type AbigenLog interface {
	Topic() common.Hash
}
