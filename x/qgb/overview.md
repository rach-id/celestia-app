# Overview

The Quantum Gravity Bridge (QGB) is a one way bridge from Celestia to EVM chains. It provides a way for rollups using Celestia for DA, and an EVM chain as a settlment layer, to prove on-chain that the rollup data was correctly posted to Celestia and verify fraud proofs otherwise. This type of rollups is called Celestiums and is discussed in the [Quantum Gravity Bridge: Secure Off-Chain Data Availability for Ethereum L2s with Celestia](https://blog.celestia.org/celestiums) blog post.

The QGB implementation consists of three components: The [state machine](https://github.com/celestiaorg/celestia-app/tree/main/x/qgb), the [orchestrator-relayer](https://github.com/celestiaorg/orchestrator-relayer), and the [QGB smart contract](https://github.com/celestiaorg/quantum-gravity-bridge).

## [State machine](https://github.com/celestiaorg/celestia-app/tree/main/x/qgb)

The state machine is the `qgb` module implementation. It is responsible for creating [attestations](https://github.com/celestiaorg/celestia-app/blob/main/x/qgb/types/attestation.go#L10-L18) which are signed by [orchestrators](https://github.com/celestiaorg/orchestrator-relayer/blob/main/docs/orchestrator.md) and then submitted to the QGB smart contract, deployed to some EVM chain, using [relayers](https://github.com/celestiaorg/orchestrator-relayer/blob/main/docs/relayer.md).

There are two types of [attestations](https://github.com/celestiaorg/celestia-app/blob/main/x/qgb/types/attestation.go#L10-L18): [valsets](https://github.com/celestiaorg/celestia-app/blob/376a1d4c0f321f12ba78279d2bd34fc6cb5e6dc2/proto/celestia/qgb/v1/types.proto#L18-L33) and [data commitments](https://github.com/celestiaorg/celestia-app/blob/376a1d4c0f321f12ba78279d2bd34fc6cb5e6dc2/proto/celestia/qgb/v1/types.proto#L35-L55).

All attestations have a [`nonce`](https://github.com/celestiaorg/celestia-app/blob/8ae6a84b2c99e55625bbe99f70db1e5a985c9675/x/qgb/types/attestation.go#L16) field that defines the order in which the attestations are generated. This nonce is stored in the QGB smart contract as per [ADR-004](https://github.com/celestiaorg/celestia-app/blob/main/docs/architecture/adr-004-qgb-relayer-security.md#decision), and is used to order attestations submissions.

### Valsets

A valset represents a validator set change. It contains a list of validators' EVM addresses along with their QGB staking power (link). It allows tracking the validator set changes inside the QGB smart contract and verify the signatures accordingly.

A valset is [generated](https://github.com/celestiaorg/celestia-app/blob/34d725993a3b2c7cbbf6e62c83bbfd90ad94657e/x/qgb/abci.go#L84-L135) inside the state machine. It is then queried, signed by orchestrators, and submitted to the QGB P2P network. After more than 2/3rds of the validator set have submitted their signatures, it is relayed to the QGB smart contract along with the signatures to be [verified](https://github.com/celestiaorg/quantum-gravity-bridge/blob/3cef3f5dfd37c3086fa40a6324f144595726dc16/src/QuantumGravityBridge.sol#L172-L211) and eventually [stored](https://github.com/celestiaorg/quantum-gravity-bridge/blob/3cef3f5dfd37c3086fa40a6324f144595726dc16/src/QuantumGravityBridge.sol#L266-L268).

The QGB smart contract keeps track of the [last validator set hash](https://github.com/celestiaorg/quantum-gravity-bridge/blob/3cef3f5dfd37c3086fa40a6324f144595726dc16/src/QuantumGravityBridge.sol#L44-L45) and its corresponding [power threshold](https://github.com/celestiaorg/quantum-gravity-bridge/blob/3cef3f5dfd37c3086fa40a6324f144595726dc16/src/QuantumGravityBridge.sol#L46-L47). This way, the contract will always be able to verify if attestations were signed using the correct validator set, and if the provided signatures represent a majority.

### Data commitments

A data commitment is an attestation type representing a request to commit over a set of blocks. It provides an end exclusive range of blocks for orchestrators to sign over and propagate in the QGB P2P network. The range is defined by the param [`DataCommitmentWindow`](https://github.com/celestiaorg/celestia-app/blob/fc83b04c3a5638ac8d415770e38a4046b84fa128/x/qgb/keeper/keeper_data_commitment.go#L44-L50).

The data commitment contains a data root tuple root, aka commitment, queried from core using the [`DataCommitment`](https://github.com/celestiaorg/celestia-core/blob/a1b07a1e6c77595466da9c61b37c83b4769b47e7/rpc/core/blocks.go#L177C1-L194) query. It is the merkelized commitment over the set of [data root tuples](https://github.com/celestiaorg/quantum-gravity-bridge/blob/3cef3f5dfd37c3086fa40a6324f144595726dc16/src/DataRootTuple.sol#L4-L17) defined by an end exclusive range of blocks. This commitment attests to the [blocks data](https://github.com/celestiaorg/celestia-core/blob/a1b07a1e6c77595466da9c61b37c83b4769b47e7/rpc/core/blocks.go#L529), the [heights](https://github.com/celestiaorg/celestia-core/blob/a1b07a1e6c77595466da9c61b37c83b4769b47e7/rpc/core/blocks.go#L528) and the [square sizes](https://github.com/celestiaorg/celestia-core/blob/a1b07a1e6c77595466da9c61b37c83b4769b47e7/rpc/core/blocks.go#L530) of all the blocks in that range which allows generating merkle inclusion proofs for any blob in any block.

When an orchestrator sees a newly generated data commitment, it queries the previous valset and checks whether it's part of its validator set. Then, the orchestrator signs the new data commitment and submits that signature to the [QGB P2P network](https://github.com/celestiaorg/orchestrator-relayer/pull/66). Otherwise, it ignores it and waits for new attestations.

After the relayer finds more than 2/3rd signatures of that data commitment, it relays the commitment along with the signatures to the QGB smart contract where they get [verified](https://github.com/celestiaorg/quantum-gravity-bridge/blob/3cef3f5dfd37c3086fa40a6324f144595726dc16/src/QuantumGravityBridge.sol#L172-L211). Then, the smart contract [saves](https://github.com/celestiaorg/quantum-gravity-bridge/blob/3cef3f5dfd37c3086fa40a6324f144595726dc16/src/QuantumGravityBridge.sol#L331-L332) the commitment to the [state](https://github.com/celestiaorg/quantum-gravity-bridge/blob/3cef3f5dfd37c3086fa40a6324f144595726dc16/src/QuantumGravityBridge.sol#L50-L51) allowing merkle inclusion proofs verification on chain for any blob posted to any committed block.

## [Orchestrator-relayer](https://github.com/celestiaorg/orchestrator-relayer)

The [orchestrator-relayer](https://github.com/celestiaorg/orchestrator-relayer) contains the implementation of the QGB orchestrator and relayer.

### [Orchestrator](https://github.com/celestiaorg/orchestrator-relayer/blob/main/docs/orchestrator.md)

An [orchestrator](https://github.com/celestiaorg/orchestrator-relayer/blob/main/docs/orchestrator.md) is the software responsible for querying the state machine for new attestations, as described above, sign them, and then submit them to the QGB P2P network.

At startup, it [loads](https://github.com/celestiaorg/orchestrator-relayer/blob/main/docs/orchestrator.md#evm-key) the EVM private key corresponding to the address used when creating the validator. Then, it uses it to sign the attestations digests before submitting them to the P2P network.

An attestation digest is a bytes array containing a digest of the attestation relevant information. More on this in the [hashes formats](#hashes-format) section.

The orchestrator generally needs access to the validator's RPC/gRPC endpoints. However, it still can use public ones if needed. Its only hard requirement is having access to the specific private key for the target validator. Otherwise, the signatures will be invalid and the validator might get slashed.

### [Relayer](https://github.com/celestiaorg/orchestrator-relayer/blob/main/docs/orchestrator.md)

A [relayer](https://github.com/celestiaorg/orchestrator-relayer/blob/main/docs/orchestrator.md) is the software responsible for querying the signatures of a validator set from the P2P network and aggregate them into a format that the [QGB smart contract](https://github.com/celestiaorg/quantum-gravity-bridge/blob/3cef3f5dfd37c3086fa40a6324f144595726dc16/src/QuantumGravityBridge.sol) understands.

It uses the previous valset to that attestation to know which validators should sign. Then, it looks for all of those signatures.

When the relayer finds more than 2/3rds of the signatures, it immediately relays them to the QGB smart contract to be persisted, and starts again.

For a [QGB smart contract](https://github.com/celestiaorg/quantum-gravity-bridge/blob/3cef3f5dfd37c3086fa40a6324f144595726dc16/src/QuantumGravityBridge.sol) not to halt, it needs at least one relayer relaying signatures to it regularly. Otherwise, the QGB contract will be out of sync and will not be able to commit to new data.

## [QGB smart contract](https://github.com/celestiaorg/quantum-gravity-bridge)

The [QGB smart contract](https://github.com/celestiaorg/quantum-gravity-bridge/blob/3cef3f5dfd37c3086fa40a6324f144595726dc16/src/QuantumGravityBridge.sol) is the source of truth for Celestiums. It allows proving/verifying that data was posted to the Celestia blockchain.

In order to reflect the Celestia chain data, the [QGB smart contract](https://github.com/celestiaorg/quantum-gravity-bridge/blob/3cef3f5dfd37c3086fa40a6324f144595726dc16/src/QuantumGravityBridge.sol) keeps track of the validator set changes, via valset updates, and commits to batches of blocks information, via data commitments.

### Validator set changes

In order to submit a validator set change, the QGB smart contract provides the [`updateValidatorSet()`](https://github.com/celestiaorg/quantum-gravity-bridge/blob/3cef3f5dfd37c3086fa40a6324f144595726dc16/src/QuantumGravityBridge.sol#L213-L273) `external` method that takes the previous valset nonce, the new one's nonce, its power threshold and its hash, along with the actual validator set and the corresponding signatures, as `calldata` to be verified. Then, it [persists](https://github.com/celestiaorg/quantum-gravity-bridge/blob/3cef3f5dfd37c3086fa40a6324f144595726dc16/src/QuantumGravityBridge.sol#L266-L268) the nonce, the valset hash and the threshold in state so they can be used for future valset and data commitment updates.

### Batches

The batches in the QGB smart contract refer to the `data root tuple root`s described above. These are submitted using the [`submitDataRootTupleRoot()`](https://github.com/celestiaorg/quantum-gravity-bridge/blob/3cef3f5dfd37c3086fa40a6324f144595726dc16/src/QuantumGravityBridge.sol#L275-L337) `external` method. This latter takes the new batch nonce, its corresponding valset, the `data root tuple root`, along with the actual validator set and their corresponding signatures as `calldata`. Then, it verifies the signature and [persists](https://github.com/celestiaorg/quantum-gravity-bridge/blob/3cef3f5dfd37c3086fa40a6324f144595726dc16/src/QuantumGravityBridge.sol#L331-L332) the new `data root tuple root` to the state along with the new nonce.

### Hashes format

The digest created/verified in the QGB smart contract follow the [EIP-712](https://eips.ethereum.org/EIPS/eip-712) standard for hashing data.

#### Valset digest

A valset digest is created inside the [`domainSeparateValidatorSetHash()`](https://github.com/celestiaorg/quantum-gravity-bridge/blob/3cef3f5dfd37c3086fa40a6324f144595726dc16/src/QuantumGravityBridge.sol#L137-L154) method. It is the `keccak256` hash of the concatenation of the following fields:

- `VALIDATOR_SET_HASH_DOMAIN_SEPARATOR`: which is defined as a [constant](https://github.com/celestiaorg/quantum-gravity-bridge/blob/3cef3f5dfd37c3086fa40a6324f144595726dc16/src/Constants.sol#L4-L6).
- The valset nonce: the [universal nonce](https://github.com/celestiaorg/celestia-app/blob/main/docs/architecture/adr-004-qgb-relayer-security.md#decision) of the attestation representing that validator set change.
- The power threshold: the threshold defining 2/3rds of the validator set.
- The validator set hash: The keccak256 hash of the validator set which is calculated using the [`computeValidatorSetHash()`](https://github.com/celestiaorg/quantum-gravity-bridge/blob/3cef3f5dfd37c3086fa40a6324f144595726dc16/src/QuantumGravityBridge.sol#L131-L135) method.

#### Data commitment digest

A data commitment digest is created inside the [`domainSeparateDataRootTupleRoot()`](https://github.com/celestiaorg/quantum-gravity-bridge/blob/3cef3f5dfd37c3086fa40a6324f144595726dc16/src/QuantumGravityBridge.sol#L156-L170) method. It is the `keccak256` hash of the concatenation of the following fields:

- `DATA_ROOT_TUPLE_ROOT_DOMAIN_SEPARATOR`: which is defined as a [constant](https://github.com/celestiaorg/quantum-gravity-bridge/blob/3cef3f5dfd37c3086fa40a6324f144595726dc16/src/Constants.sol#L8-L10).
- The data commitment nonce: the [universal nonce](https://github.com/celestiaorg/celestia-app/blob/main/docs/architecture/adr-004-qgb-relayer-security.md#decision) of the attestation representing that data commitment.
- The data root tuple root: which is the commitment over the set of blocks defined [above](#data-commitments).

### Signatures

The signature scheme used for signing the above hashes follow the [EIP-191](https://eips.ethereum.org/EIPS/eip-191) signing standard. It uses the `ECDSA` algorithm with the `secp256k1` curve. So, the orchestrator uses the keystore to [generate]((https://github.com/celestiaorg/orchestrator-relayer/blob/09ebfdc312c0d9e08856fb98cfd089e956ab7f3a/evm/ethereum_signature.go#L18-L28)) these signatures.

The output signature is in the `[R || S || V]` format where `V` is `0` or `1`. This is defined in the QGB smart contract using the [Signature](https://github.com/celestiaorg/quantum-gravity-bridge/blob/3cef3f5dfd37c3086fa40a6324f144595726dc16/src/QuantumGravityBridge.sol#L17-L21) struct.

These signatures are then verified in the smart contract using the [`verifySig()`](https://github.com/celestiaorg/quantum-gravity-bridge/blob/3cef3f5dfd37c3086fa40a6324f144595726dc16/src/QuantumGravityBridge.sol#L124-L129) method.

## Security assumptions

The QGB security relies on Celestia validator set security, which assumes that the majority of the participating validator aren't Byzantine and behave honestly. This assumption ensures that the honest nodes have control over the consensus outcome and can overpower any malicious behavior from the Byzantine nodes.

So, if more than 1/3rd of the validator set stops running their orchestrators,then the QGB halts. And, if more than 2/3rds sign invalid data, then the QGB contract will commit to invalid data and we will need to redeploy the smart contract and social slash the dishonest validators.

## Slashing

We still don't support slashing for equivocation, liveness  or double signatures. However, if anything were to happen to the bridge, we would be able to social slash the corrupt validators and redeploy the contract.

## Proofs

To prove that data was posted to an EVM chain, we have the following method: [`verifyAttestation()`](https://github.com/celestiaorg/quantum-gravity-bridge/blob/3cef3f5dfd37c3086fa40a6324f144595726dc16/src/QuantumGravityBridge.sol#L339-L358). This allows to verify that a data root tuple was committed to by the QGB  smart contract.

To prove that data was posted to Celestia, an extra contract will need to be used to verify the proofs from shares to the data root (TBD). These proofs are as follow:

- NMT proof from the shares to the row roots.
- The row roots proof to the data root.
- the data root tuple proof to the data root tuple root, which is verified using the [`verifyAttestation()`](https://github.com/celestiaorg/quantum-gravity-bridge/blob/3cef3f5dfd37c3086fa40a6324f144595726dc16/src/QuantumGravityBridge.sol#L339-L358) method.