package main

import (
	"time"
)

const (
	// ****************
	// ** Scheduling **
	roundTime      = 5 * time.Second
	fuzzRoundNBase = 5

	// *****************************
	// ** Evolutionnary Algorithm **
	// Turn evolutionnary algorithm off for experiment.
	useEvoA = false

	// *****************************************
	// ** pcaFitFunc initialization constants **
	pcaInitTime  = 2 * time.Second
	initQueueMax = 100

	// ***************************
	// ** dynamic PCA constants **
	pcaInitDim = 10

	// Phase 2
	phase2Dur = time.Second
	// Phase 3
	phase3Dur     = phase2Dur
	convCritFloor = 0.05 // Floor to apply rotation.

	bucketSensitiveness = 5 // How many buckets per std in histogram.

	// *************
	// ** Verbose **
	printTickT = 3 * time.Second

	// ************************
	// ** Distance parameter **
	regulizer = 0.1

	// ************
	// ** System **
	deactivateHyperthread = true
)

var fuzzRoundN = fuzzRoundNBase

func init() {
	if !useEvoA {
		fuzzRoundN *= 3
	}
}
