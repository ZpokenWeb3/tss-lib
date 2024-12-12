// Copyright © 2019 Binance
//
// This file is part of Binance. The full Binance copyright notice, including
// terms governing use, modification, and redistribution, is contained in the
// file LICENSE at the root of the source code distribution tree.

package signing

import (
	"math/big"

	"github.com/agl/ed25519/edwards25519"
	"github.com/bnb-chain/tss-lib/v2/common"
	"github.com/pkg/errors"

	"github.com/bnb-chain/tss-lib/v2/crypto"
	"github.com/bnb-chain/tss-lib/v2/crypto/commitments"
	"github.com/bnb-chain/tss-lib/v2/crypto/poseidon"
	"github.com/bnb-chain/tss-lib/v2/tss"
)

// flattenByteSlices concatenates slices of bytes into a single slice
func flattenByteSlices(slices [][]byte) []byte {
	totalLength := 0
	for _, slice := range slices {
		if len(slice) == 0 {
			panic("empty slice detected in Poseidon inputs")
		}
		totalLength += len(slice)
	}

	flattened := make([]byte, totalLength)
	offset := 0
	for _, slice := range slices {
		copy(flattened[offset:], slice)
		offset += len(slice)
	}
	return flattened
}

func (round *round3) Start() *tss.Error {
	if round.started {
		return round.WrapError(errors.New("round already started"))
	}

	round.number = 3
	round.started = true
	round.resetOK()

	// 1. Initialize R
	var R edwards25519.ExtendedGroupElement
	riBytes := bigIntToEncodedBytes(round.temp.ri)
	edwards25519.GeScalarMultBase(&R, riBytes)

	// 2-6. Compute R
	i := round.PartyID().Index
	for j, Pj := range round.Parties().IDs() {
		if j == i {
			continue
		}

		ContextJ := common.AppendBigIntToBytesSlice(round.temp.ssid, big.NewInt(int64(j)))
		msg := round.temp.signRound2Messages[j]
		r2msg := msg.Content().(*SignRound2Message)
		cmtDeCmt := commitments.HashCommitDecommit{C: round.temp.cjs[j], D: r2msg.UnmarshalDeCommitment()}
		ok, coordinates := cmtDeCmt.DeCommit()
		if !ok {
			return round.WrapError(errors.New("de-commitment verify failed"))
		}
		if len(coordinates) != 2 {
			return round.WrapError(errors.New("length of de-commitment should be 2"))
		}

		Rj, err := crypto.NewECPoint(round.Params().EC(), coordinates[0], coordinates[1])
		Rj = Rj.EightInvEight()
		if err != nil {
			return round.WrapError(errors.Wrapf(err, "NewECPoint(Rj)"), Pj)
		}
		proof, err := r2msg.UnmarshalZKProof(round.Params().EC())
		if err != nil {
			return round.WrapError(errors.New("failed to unmarshal Rj proof"), Pj)
		}
		ok = proof.Verify(ContextJ, Rj)
		if !ok {
			return round.WrapError(errors.New("failed to prove Rj"), Pj)
		}

		extendedRj := ecPointToExtendedElement(round.Params().EC(), Rj.X(), Rj.Y(), round.Rand())
		R = addExtendedElements(R, extendedRj)
	}

	// 7. Compute lambda using Poseidon
	var encodedR [32]byte
	R.ToBytes(&encodedR)
	encodedPubKey := ecPointToEncodedBytes(round.key.EDDSAPub.X(), round.key.EDDSAPub.Y())

	// Prepare inputs for Poseidon
	poseidonInputs := [][]byte{encodedR[:], encodedPubKey[:]}
	if round.temp.fullBytesLen == 0 {
		poseidonInputs = append(poseidonInputs, round.temp.m.Bytes())
	} else {
		mBytes := make([]byte, round.temp.fullBytesLen)
		round.temp.m.FillBytes(mBytes)
		poseidonInputs = append(poseidonInputs, mBytes)
	}

	// Perform Poseidon hashing
	poseidonHash, err := poseidon.HashBytes(flattenByteSlices(poseidonInputs))
	if err != nil {
		return round.WrapError(errors.Wrap(err, "Poseidon hashing failed"))
	}

	// Convert Poseidon hash to a [64]byte array
	var lambda [64]byte
	copy(lambda[:], poseidonHash.Bytes())
	common.Logger.Infof("Poseidon Hash (lambda): %x", lambda)

	// Reduce the hash output to a scalar
	var lambdaReduced [32]byte
	edwards25519.ScReduce(&lambdaReduced, &lambda)

	// 8. Compute si
	var localS [32]byte
	edwards25519.ScMulAdd(&localS, &lambdaReduced, bigIntToEncodedBytes(round.temp.wi), riBytes)
	common.Logger.Infof("Reduced lambda: %x", lambdaReduced)

	// 9. Store r3 message pieces
	round.temp.si = &localS
	round.temp.r = encodedBytesToBigInt(&encodedR)
	common.Logger.Infof("Computed si: %x", localS)
	common.Logger.Infof("Inputs to Poseidon hash: R=%x, PubKey=%x, Message=%x", encodedR[:], encodedPubKey[:], round.temp.m.Bytes())

	// 10. Broadcast si to other parties
	r3msg := NewSignRound3Message(round.PartyID(), encodedBytesToBigInt(&localS))
	round.temp.signRound3Messages[round.PartyID().Index] = r3msg
	round.out <- r3msg

	return nil
}

func (round *round3) Update() (bool, *tss.Error) {
	ret := true
	for j, msg := range round.temp.signRound3Messages {
		if round.ok[j] {
			continue
		}
		if msg == nil || !round.CanAccept(msg) {
			ret = false
			continue
		}
		round.ok[j] = true
	}
	return ret, nil
}

func (round *round3) CanAccept(msg tss.ParsedMessage) bool {
	if _, ok := msg.Content().(*SignRound3Message); ok {
		return msg.IsBroadcast()
	}
	return false
}

func (round *round3) NextRound() tss.Round {
	round.started = false
	return &finalization{round}
}
