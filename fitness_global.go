package main

import (
	"fmt"

	"time"
)

// *****************************************************************************
// **************************** DevNull Channel ********************************

var devNullFitChan chan runMeta

func init() {
	devNullFitChan = make(chan runMeta)
	go func() {
		for _ = range devNullFitChan {
		}
	}()
}

// *****************************************************************************
// **************************** Global Fitness *********************************

type globalFitness struct {
	*brCovFitFunc

	ticker *time.Ticker
}

func makeGlbFitness() chan runMeta {
	fitChan := make(chan runMeta, 1000)

	glbFit := globalFitness{
		brCovFitFunc: newBrCovFitFunc(),
		ticker:       time.NewTicker(time.Second),
	}
	go glbFit.listen(fitChan)

	return fitChan
}

func (glbFit globalFitness) listen(fitChan chan runMeta) {
	_, sigChan := intChans.add() // Get notified when interrupted.

	fuzzContinue := true
	for fuzzContinue {
		select {
		case _ = <-sigChan:
			fuzzContinue = false
			break
		case _ = <-glbFit.ticker.C:
			fmt.Printf("Global fitness: %v.\n", glbFit.brCovFitFunc)

		case runInfo := <-fitChan:
			_ = glbFit.isFit(runInfo)
		}
	}

	glbFit.ticker.Stop()
}
