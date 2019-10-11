package main

type fitnessFunc interface {
	isFit(runInfo runMeta) bool
}
