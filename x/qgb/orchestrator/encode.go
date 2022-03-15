package orchestrator

import (
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/celestiaorg/celestia-app/x/qgb/types"
	wrappers "github.com/celestiaorg/quantum-gravity-bridge/ethereum/solidity/wrappers/QuantumGravityBridge.sol"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethcmn "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var qgbContractABI abi.ABI

func init() {
	// error case here should not occur outside of testing since the above is a constant
	// todo: update the abi used
	contractAbi, abiErr := abi.JSON(strings.NewReader(wrappers.QuantumGravityBridgeMetaData.ABI))
	if abiErr != nil {
		log.Fatalln("bad ABI constant")
	}
	qgbContractABI = contractAbi
}

// EncodeValsetConfirm takes the required input data and produces the required signature to confirm a validator
// set update on the Peggy Ethereum contract. This value will then be signed before being
// submitted to Cosmos, verified, and then relayed to Ethereum
func EncodeValsetConfirm(bridgeID common.Hash, valset *types.Valset, vsHash ethcmn.Hash) common.Hash {
	checkpointBytes := []uint8("checkpoint")
	var checkpoint [32]uint8
	copy(checkpoint[:], checkpointBytes)

	// the word 'checkpoint' needs to be the same as the 'name' above in the checkpointAbiJson
	// but other than that it's a constant that has no impact on the output. This is because
	// it gets encoded as a function name which we must then discard.
	bytes, packErr := qgbContractABI.Pack(
		"checkpoint",
		bridgeID,
		checkpoint,
		big.NewInt(int64(valset.Nonce)),
		big.NewInt(int64(valset.TwoThirdsThreshold())),
		vsHash,
	)
	// this should never happen outside of test since any case that could crash on encoding
	// should be filtered above.
	if packErr != nil {
		panic(fmt.Sprintf("Error packing checkpoint! %s/n", packErr))
	}

	// we hash the resulting encoded bytes discarding the first 4 bytes these 4 bytes are the constant
	// method name 'checkpoint'. If you where to replace the checkpoint constant in this code you would
	// then need to adjust how many bytes you truncate off the front to get the output of abi.encode()
	hash := crypto.Keccak256Hash(bytes[4:])
	return hash
}

// EncodeValsetConfirm takes the required input data and produces the required signature to confirm a validator
// set update on the Peggy Ethereum contract. This value will then be signed before being
// submitted to Cosmos, verified, and then relayed to Ethereum
func EncodeDataCommitmentConfirm(bridgeID common.Hash, nonce *big.Int, commitment []byte) common.Hash {
	transactionBatchBytes := []uint8("transactionBatch")
	var transactionBatch [32]uint8
	copy(transactionBatch[:], transactionBatchBytes)

	var dataCommitment [32]uint8
	copy(dataCommitment[:], commitment)

	// the word 'transactionBatch' needs to be the same as the 'name' above in the DataCommitmentConfirmABIJSON
	// but other than that it's a constant that has no impact on the output. This is because
	// it gets encoded as a function name which we must then discard.
	bytes, packErr := qgbContractABI.Pack(
		"transactionBatch",
		bridgeID,
		transactionBatch,
		nonce,
		dataCommitment,
	)
	// this should never happen outside of test since any case that could crash on encoding
	// should be filtered above.
	if packErr != nil {
		panic(fmt.Sprintf("Error packing checkpoint! %s/n", packErr))
	}

	// we hash the resulting encoded bytes discarding the first 4 bytes these 4 bytes are the constant
	// method name 'checkpoint'. If you where to replace the checkpoint constant in this code you would
	// then need to adjust how many bytes you truncate off the front to get the output of abi.encode()
	hash := crypto.Keccak256Hash(bytes[4:])
	return hash
}

const (
	// ValsetCheckpointABIJSON checks the ETH ABI for compatibility of the Valset update message
	ValsetCheckpointABIJSON = `[{
		"name": "checkpoint",
		"stateMutability": "pure",
		"type": "function",
		"inputs": [
			{ "internalType": "bytes32",   "name": "_bridge_id",   "type": "bytes32"   },
			{ "internalType": "bytes32",   "name": "_checkpoint",  "type": "bytes32"   },
			{ "internalType": "uint256",   "name": "_valsetNonce", "type": "uint256"   },
			{ "internalType": "address[]", "name": "_validators",  "type": "address[]" },
			{ "internalType": "uint256[]", "name": "_powers",      "type": "uint256[]" },
		],
		"outputs": [
			{ "internalType": "bytes32", "name": "", "type": "bytes32" }
		]
	}]`

	DataCommitmentConfirmABIJSON = `[{
        "name": "transactionBatch",
        "stateMutability": "pure",
        "type": "function",
        "inputs": [
			{ "internalType": "bytes32", "name": "_bridge_id",            "type": "bytes32"   },
			{ "internalType": "bytes32", "name": "_checkpoint",           "type": "bytes32"   },
			{ "internalType": "uint256", "name": "_nonce",                "type": "uint256"   },
			{ "internalType": "bytes32", "name": "_data_root_tuple_root", "type": "bytes32" },
        ],
		"outputs": [
			{ "internalType": "bytes32", "name": "", "type": "bytes32" }
		]
    }]`
)
