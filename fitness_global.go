package main

import (
	"fmt"

	"time"
)

// *****************************************************************************
// **************************** DevNull Channel ********************************

var devNullFitChan chan runT

func init() {
	devNullFitChan = make(chan runT)
	go func() {
		for _ = range devNullFitChan {
		}
	}()
}

// *****************************************************************************
// **************************** Global Fitness *********************************

type globalFitness struct {
	*brCovFitFunc

	ticker   *time.Ticker
	stopChan chan struct{}
}

func makeGlbFitness(fitChan chan runT, newSeedChan chan *seedT) chan struct{} {
	glbFit := globalFitness{
		brCovFitFunc: newBrCovFitFunc(),
		ticker:       time.NewTicker(printTickT),
		stopChan:     make(chan struct{}),
	}
	go glbFit.listen(fitChan, newSeedChan)
	return glbFit.stopChan
}

func (glbFit globalFitness) listen(fitChan chan runT, newSeedChan chan *seedT) {
	_, sigChan := intChans.add() // Get notified when interrupted.

	fuzzContinue := true
	for fuzzContinue {
		select {
		case _ = <-sigChan:
			fuzzContinue = false
			break
		case _ = <-glbFit.stopChan:
			fuzzContinue = false
			break
		case _ = <-glbFit.ticker.C:
			fmt.Printf("Global fitness: %v.\n", glbFit.brCovFitFunc)

		case runInfo := <-fitChan:
			if !useEvoA {
				glbFit.isFit(runInfo)
			} else if glbFit.isFit(runInfo) {
				newSeedChan <- &seedT{runT: runInfo}
			}
		}
	}

	glbFit.ticker.Stop()
}
