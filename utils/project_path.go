package utils

import (
	"path/filepath"
	"runtime"
)

var (
	_, b, _, _ = runtime.Caller(0)
	// ProjectRoot Root folder of this project
	ProjectRoot = filepath.Join(filepath.Dir(b), "/..")
	// PresetRoot root folder for environments preset
	PresetRoot = filepath.Join(ProjectRoot, "preset")
	// ContractsDir path to our contracts
	ContractsDir = filepath.Join(ProjectRoot, "contracts")
	// EthereumContractsDir path to our celo contracts
	EthereumContractsDir = filepath.Join(ContractsDir, "celo")
	// RemoteRunnerConfigLocation is the path to the remote runner config
	RemoteRunnerConfigLocation = filepath.Join(ProjectRoot, "remote_runner_config.yaml")
)
