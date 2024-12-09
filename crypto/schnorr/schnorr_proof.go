// Copyright © 2019 Binance
//
// This file is part of Binance. The full Binance copyright notice, including
// terms governing use, modification, and redistribution, is contained in the
// file LICENSE at the root of the source code distribution tree.

package schnorr

import (
	"errors"
	"fmt"
	"io"
	"math/big"

	"github.com/bnb-chain/tss-lib/v2/common"
	"github.com/bnb-chain/tss-lib/v2/crypto"
)

type (
	ZKProof struct {
		Alpha *crypto.ECPoint
		T     *big.Int
	}

	ZKVProof struct {
		Alpha *crypto.ECPoint
		T, U  *big.Int
	}
)

// NewZKProof constructs a new Schnorr ZK proof of knowledge of the discrete logarithm (GG18Spec Fig. 16)
func NewZKProofBJJ(Session []byte, x *big.Int, X *crypto.ECPoint, rand io.Reader) (*ZKProof, error) {
	// fmt.Printf("\n NewZKProof: x %d, X %d, rand %d\n", x, X, rand)
	// if x == nil || X == nil || !X.ValidateBasic() {
	if !X.ValidateBasicBJJ() {
		return nil, errors.New("BJJ ZKProof constructor received nil or invalid value(s)")
	}
	ec := X.Curve()
	ecParams := ec.Params()
	q := ecParams.N
	g := crypto.NewECPointNoCurveCheck(ec, ecParams.Gx, ecParams.Gy) // already on the curve.

	a := common.GetRandomPositiveInt(rand, q)
	alpha := crypto.ScalarBaseMultBJJ(ec, a)

	var c *big.Int
	{
		cHash := common.SHA512_256i_TAGGED(Session, X.X(), X.Y(), g.X(), g.Y(), alpha.X(), alpha.Y())
		c = common.RejectionSample(q, cHash)
	}
	t := new(big.Int).Mul(c, x)
	t = common.ModInt(q).Add(a, t)

	return &ZKProof{Alpha: alpha, T: t}, nil
}

// NewZKProof constructs a new Schnorr ZK proof of knowledge of the discrete logarithm (GG18Spec Fig. 16)
func NewZKProof(Session []byte, x *big.Int, X *crypto.ECPoint, rand io.Reader) (*ZKProof, error) {
	fmt.Printf("\n NewZKProof: x %d, X %d, rand %d\n", x, X, rand)
	if x == nil || X == nil || !X.ValidateBasic() {
		return nil, errors.New("ZKProof constructor received nil or invalid value(s)")
	}
	ec := X.Curve()
	ecParams := ec.Params()
	q := ecParams.N
	g := crypto.NewECPointNoCurveCheck(ec, ecParams.Gx, ecParams.Gy) // already on the curve.

	a := common.GetRandomPositiveInt(rand, q)
	alpha := crypto.ScalarBaseMult(ec, a)

	var c *big.Int
	{
		cHash := common.SHA512_256i_TAGGED(Session, X.X(), X.Y(), g.X(), g.Y(), alpha.X(), alpha.Y())
		c = common.RejectionSample(q, cHash)
	}
	t := new(big.Int).Mul(c, x)
	t = common.ModInt(q).Add(a, t)

	return &ZKProof{Alpha: alpha, T: t}, nil
}

// NewZKProof verifies a new Schnorr ZK proof of knowledge of the discrete logarithm (GG18Spec Fig. 16)
func (pf *ZKProof) Verify(Session []byte, X *crypto.ECPoint) bool {
	if pf == nil || !pf.ValidateBasic() {
		return false
	}
	ec := X.Curve()
	ecParams := ec.Params()
	q := ecParams.N
	g := crypto.NewECPointNoCurveCheck(ec, ecParams.Gx, ecParams.Gy)

	var c *big.Int
	{
		cHash := common.SHA512_256i_TAGGED(Session, X.X(), X.Y(), g.X(), g.Y(), pf.Alpha.X(), pf.Alpha.Y())
		c = common.RejectionSample(q, cHash)
	}
	tG := crypto.ScalarBaseMult(ec, pf.T)
	Xc := X.ScalarMult(c)
	aXc, err := pf.Alpha.Add(Xc)
	if err != nil {
		return false
	}
	return aXc.X().Cmp(tG.X()) == 0 && aXc.Y().Cmp(tG.Y()) == 0
}

func (pf *ZKProof) ValidateBasic() bool {
	return pf.T != nil && pf.Alpha != nil
}

// NewZKProof constructs a new Schnorr ZK proof of knowledge s_i, l_i such that V_i = R^s_i, g^l_i (GG18Spec Fig. 17)
func NewZKVProof(Session []byte, V, R *crypto.ECPoint, s, l *big.Int, rand io.Reader) (*ZKVProof, error) {
	if V == nil || R == nil || s == nil || l == nil || !V.ValidateBasic() || !R.ValidateBasic() {
		return nil, errors.New("ZKVProof constructor received nil value(s)")
	}
	ec := V.Curve()
	ecParams := ec.Params()
	q := ecParams.N
	g := crypto.NewECPointNoCurveCheck(ec, ecParams.Gx, ecParams.Gy)

	a, b := common.GetRandomPositiveInt(rand, q), common.GetRandomPositiveInt(rand, q)
	aR := R.ScalarMult(a)
	bG := crypto.ScalarBaseMult(ec, b)
	alpha, _ := aR.Add(bG) // already on the curve.

	var c *big.Int
	{
		cHash := common.SHA512_256i_TAGGED(Session, V.X(), V.Y(), R.X(), R.Y(), g.X(), g.Y(), alpha.X(), alpha.Y())
		c = common.RejectionSample(q, cHash)
	}
	modQ := common.ModInt(q)
	t := modQ.Add(a, new(big.Int).Mul(c, s))
	u := modQ.Add(b, new(big.Int).Mul(c, l))

	return &ZKVProof{Alpha: alpha, T: t, U: u}, nil
}

func (pf *ZKVProof) Verify(Session []byte, V, R *crypto.ECPoint) bool {
	if pf == nil || !pf.ValidateBasic() {
		return false
	}
	ec := V.Curve()
	ecParams := ec.Params()
	q := ecParams.N
	g := crypto.NewECPointNoCurveCheck(ec, ecParams.Gx, ecParams.Gy)

	var c *big.Int
	{
		cHash := common.SHA512_256i_TAGGED(Session, V.X(), V.Y(), R.X(), R.Y(), g.X(), g.Y(), pf.Alpha.X(), pf.Alpha.Y())
		c = common.RejectionSample(q, cHash)
	}
	tR := R.ScalarMult(pf.T)
	uG := crypto.ScalarBaseMult(ec, pf.U)
	tRuG, _ := tR.Add(uG) // already on the curve.

	Vc := V.ScalarMult(c)
	aVc, err := pf.Alpha.Add(Vc)
	if err != nil {
		return false
	}
	return tRuG.X().Cmp(aVc.X()) == 0 && tRuG.Y().Cmp(aVc.Y()) == 0
}

func (pf *ZKVProof) ValidateBasic() bool {
	return pf.Alpha != nil && pf.T != nil && pf.U != nil && pf.Alpha.ValidateBasic()
}
