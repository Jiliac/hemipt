package main

import (
	"time"
)

const (
	// ****************
	// ** Scheduling **
	roundTime      = 5 * time.Second
	fuzzRoundNBase = 3

	// **********************
	// ** Input Generation **
	mutationRatio = 1.0 / 100

	// *****************************************
	// ** pcaFitFunc initialization constants **
	pcaInitTime  = 2 * time.Second
	initQueueMax = 100
	maxPCADimN   = 200

	// ***************************
	// ** dynamic PCA constants **
	pcaInitDim = 10

	// Phase 2
	phase2Dur = time.Second
	// Phase 3
	phase3Dur     = phase2Dur
	convCritFloor = 0.05 // Floor to apply rotation.

	bucketSensitiveness = 10 // How many buckets per std in histogram.

	// *************
	// ** Verbose **
	printTickT = 3 * time.Second

	// ************************
	// ** Distance parameter **
	regulizer = 0.1

	// ************
	// ** System **
	deactivateHyperthread = true

	// *****************
	// ** Experiments **
	useEvoA = true  // Turn evolutionnary algorithm off for experiment.
	logFreq = false // Log hash frequencies for MLE divergence estimation.
	//
	// Based on all the seeds and their PCA, get a global base. Record
	// historgram on this base and compute divergences.
	doDivPhase = true
	//
	trackGlbFreqs = true
)

var fuzzRoundN = fuzzRoundNBase
var didDivPhase bool

func init() {
	if logFreq {
		fuzzRoundN = 12
	}
}
