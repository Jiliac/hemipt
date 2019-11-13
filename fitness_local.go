package main

import (
	"fmt"

	"time"
)

// The fitness function interfaces are intended to do the "heavy analysis" of a
// seed and to do in a single thread/goroutine. (This way we don't lose track of
// which CPU it is executed on).
// These interfaces are _local_.
type fitnessFunc interface {
	isFit(runInfo runT) bool
	String() string
}

// *****************************************************************************
// ************************** Fitness Multiplexer ******************************

type fitnessMultiplexer []fitnessFunc

func (fm fitnessMultiplexer) isFit(runInfo runT) (fit bool) {
	fits := make([]bool, len(fm))
	for i, ff := range fm {
		fits[i] = ff.isFit(runInfo)
	}
	//
	for _, fitI := range fits {
		fit = fit || fitI
	}
	return fit
}
func (fm fitnessMultiplexer) String() (str string) {
	str = "[\n"
	for i, ff := range fm {
		str += fmt.Sprintf("%d:\t%s\n", i, ff.String())
	}
	str += "]"
	return str
}

// *****************************************************************************
// ****************************** Mock fitness *********************************

type falseFitFunc struct{}
type trueFitFunc struct{}

func (falseFitFunc) isFit(runT) bool { return false }
func (falseFitFunc) String() string  { return "always false" }
func (trueFitFunc) isFit(runT) bool  { return true }
func (trueFitFunc) String() string   { return "always true" }

// *****************************************************************************
// **************************** Branch Coverage ********************************

type brCovFitFunc struct {
	brMap  map[int]struct{}
	hashes map[uint64]struct{}
	brList []int
	execN  int
}

func newBrCovFitFunc() *brCovFitFunc {
	return &brCovFitFunc{
		brMap:  make(map[int]struct{}),
		hashes: make(map[uint64]struct{}),
	}
}

func (fitFunc *brCovFitFunc) isFit(runInfo runT) (fit bool) {
	// This one is just for log. Could/should delete for performance.
	fitFunc.hashes[runInfo.hash] = struct{}{}

	fitFunc.execN++

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
	return fmt.Sprintf("%d branch and,\t%.3v hashes,\t #exec: %.3v",
		len(fitFunc.brMap),
		float64(len(fitFunc.hashes)),
		float64(fitFunc.execN),
	)
}

// *****************************************************************************
// ****************************** PCA Fitness **********************************
// ATM (October 2019), this is going to be very experimentative. Just care about
// my current case where only several seeds are constantly fuzzed (kind of a
// blackbox mode). If/when expanded to future usage, may need significant
// modification or even just rewriting all.

type pcaFitFunc struct {
	// Init
	initializing bool
	initTimer    *time.Timer
	queue        [][]byte

	hashes  map[uint64]struct{}
	hashesF map[uint64]byte

	dynpca *dynamicPCA
}

func newPCAFitFunc() *pcaFitFunc {
	pff := &pcaFitFunc{
		initializing: true,
		initTimer:    time.NewTimer(pcaInitTime),
		hashes:       make(map[uint64]struct{}),
		hashesF:      make(map[uint64]byte),
	}
	return pff
}

func (pff *pcaFitFunc) isFit(runInfo runT) (fit bool) {
	select {
	case _ = <-pff.initTimer.C:
		pff.endInit()
	default:
		if len(pff.queue) >= initQueueMax {
			pff.initTimer.Stop()
			pff.endInit()
		}
	}

	pff.logFreq(runInfo.hash) // For experiment
	if pff.initializing {
		if _, ok := pff.hashes[runInfo.hash]; !ok {
			pff.queue = append(pff.queue, runInfo.trace)
		}
		pff.hashes[runInfo.hash] = struct{}{}
		return fit

	} else {
		pff.hashes[runInfo.hash] = struct{}{}
	}

	pff.dynpca.newSample(runInfo.trace)

	return fit
}
func (pff *pcaFitFunc) logFreq(hash uint64) {
	if !logFreq {
		return
	}

	if f, ok := pff.hashesF[hash]; !ok {
		pff.hashesF[hash] = 1
	} else if f != 0xff {
		pff.hashesF[hash]++
	}
}

func (pff *pcaFitFunc) endInit() {
	if len(pff.queue) < pcaInitDim {
		pff.initTimer = time.NewTimer(3 * pcaInitTime)
		return
	}
	var ok bool
	ok, pff.dynpca = newDynPCA(pff.queue)
	if ok {
		pff.initializing = false
	} else {
		pff.dynpca = nil
	}
	pff.queue = nil
}

func (pff *pcaFitFunc) String() string {
	return pff.dynpca.String()
}
