package main

type fitnessFunc interface {
	isFit(runInfo runMeta) bool
}

// *****************************************************************************
// ****************************** Mock fitness *********************************

var devNullFitChan chan runMeta

func init() {
	devNullFitChan = make(chan runMeta)
	go func() {
		for _ = range devNullFitChan {
		}
	}()
}

type falseFitFunc struct{}
type trueFitFunc struct{}

func (falseFitFunc) isFit(runMeta) bool { return false }
func (trueFitFunc) isFit(runMeta) bool  { return true }
