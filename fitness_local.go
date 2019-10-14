package main

import (
	"fmt"
)

// The fitness function interfaces are intended to do the "heavy analysis" of a
// seed and to do in a single thread/goroutine. (This way we don't lose track of
// which CPU it is executed on).
// These interfaces are _local_.
type fitnessFunc interface {
	isFit(runInfo runMeta) bool
	String() string
}

// *****************************************************************************
// ****************************** Mock fitness *********************************

type falseFitFunc struct{}
type trueFitFunc struct{}

func (falseFitFunc) isFit(runMeta) bool { return false }
func (falseFitFunc) String() string     { return "always false" }
func (trueFitFunc) isFit(runMeta) bool  { return true }
func (trueFitFunc) String() string      { return "always true" }

// *****************************************************************************
// **************************** Branch Coverage ********************************

type brCovFitFunc struct {
	brMap   map[int]struct{}
	hashMap map[uint64]struct{}
	brList  []int
}

func newBrCovFitFunc() *brCovFitFunc {
	return &brCovFitFunc{
		brMap:   make(map[int]struct{}),
		hashMap: make(map[uint64]struct{}),
	}
}

func (fitFunc *brCovFitFunc) isFit(runInfo runMeta) (fit bool) {
	fitFunc.hashMap[runInfo.hash] = struct{}{}

	trace := runInfo.trace
	for i, tr := range trace {
		if tr == 0 {
			continue
		}
		if _, ok := fitFunc.brMap[i]; !ok {
			fit = true
			fitFunc.brMap[i] = struct{}{}
			fitFunc.brList = append(fitFunc.brList, i)
		}
	}

	return fit
}

func (fitFunc *brCovFitFunc) String() string {
	return fmt.Sprintf("%.3v branch and %.3v hashes",
		float64(len(fitFunc.brMap)),
		float64(len(fitFunc.hashMap)),
	)
}
