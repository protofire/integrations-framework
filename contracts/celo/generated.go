package celo

import (
	"github.com/celo-org/celo-blockchain/common"
)

// AbigenLog is an interface for abigen generated log topics
type AbigenLog interface {
	Topic() common.Hash
}

type KeeperRegistryVersion int32

const (
	RegistryVersion_1_0 KeeperRegistryVersion = iota
	RegistryVersion_1_1
	RegistryVersion_1_2
)
