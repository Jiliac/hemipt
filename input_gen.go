package main

import (
	"math/rand"
)

type inputGen interface {
	generate() (testCase []byte)
}

var (
	_ inputGen = seedCopier([]byte{})
	_ inputGen = ratioMutator{}
)

// *****************************************************************************
// ******************************** Seed Gen ***********************************
// Just a dummy input generator: always return the seed as is.

type seedCopier []byte

func (sc seedCopier) generate() []byte { return []byte(sc) }

// *****************************************************************************
// **************************** Ratio Mutator **********************************

const chunkSize = 1024

type ratioMutator struct {
	r      *rand.Rand
	ratio  float64
	seedIn []byte
}

func makeRatioMutator(seedIn []byte, ratio float64) ratioMutator {
	return ratioMutator{
		r:      rand.New(rand.NewSource(rand.Int63())),
		ratio:  ratio,
		seedIn: seedIn,
	}
}

func (rMut ratioMutator) generate() (testCase []byte) {
	n := len(rMut.seedIn)
	testCase = make([]byte, n)
	chunkN := n / chunkSize
	if n%chunkSize != 0 {
		chunkN++
	}

	chunk := make([]byte, chunkSize) // Should just memset previous one.
	for i := 0; i < chunkN; i++ {

		for j := range chunk {
			chunk[j] = 0
		}
		todo := int(rMut.ratio*8*chunkSize + rMut.r.Float64())
		for i := 0; i < todo; i++ {
			idx := rMut.r.Intn(chunkSize)
			bit := byte(1 << uint(rMut.r.Intn(8)))
			chunk[idx] ^= bit
		}

		start, end := i*chunkSize, (i+1)*chunkSize
		if end > n {
			end = n
		}
		for j := start; j < end; j++ {
			testCase[j] = rMut.seedIn[j] ^ chunk[j%chunkSize]
		}
	}

	return testCase
}
