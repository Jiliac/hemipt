package main

import "math"

type regionT struct {
	center []byte
	proj   []float64

	speciesMap map[uint64]struct{}
	speciesN   int
	sampleN    int
}

func makeRegion(center []byte, proj []float64) regionT {
	return regionT{
		center:     center,
		proj:       proj,
		speciesMap: make(map[uint64]struct{}),
	}
}

func findRegion(regions []regionT, pt []float64, hash uint64) {
	var minDist float64
	var closestRI int
	for i, r := range regions {
		var dist float64
		for j, p := range r.proj {
			diff := p - pt[j]
			dist += diff * diff
		}
		if dist < minDist {
			minDist = dist
			closestRI = i
		}
	}

	regions[closestRI].sampleN++
	if _, ok := regions[closestRI].speciesMap[hash]; !ok {
		regions[closestRI].speciesMap[hash] = struct{}{}
		regions[closestRI].speciesN++
	}
}

func (r regionT) expectedSampleReward() float64 {
	if r.speciesN == 0 {
		return 1
	}
	specN := float64(r.speciesN)
	discoveryP := specN / float64(r.sampleN)
	discoveryR := math.Log((specN + 1) / specN)
	return discoveryP * discoveryR
}
