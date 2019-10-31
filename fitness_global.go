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

	ticker *time.Ticker
}

func makeGlbFitness(fitChan chan runT) {
	glbFit := globalFitness{
		brCovFitFunc: newBrCovFitFunc(),
		ticker:       time.NewTicker(time.Second),
	}
	go glbFit.listen(fitChan)
}

func (glbFit globalFitness) listen(fitChan chan runT) {
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
