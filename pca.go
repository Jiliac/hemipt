package main

import (
	"fmt"
	"log"

	"math"
	"sort"
	"time"

	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/stat"

	"github.com/olekukonko/tablewriter"
	"strings"
)

// Phase 1 (short): fitness function collect some traces and then initialize the
// dynamicPCA. (Implemented in newDynPCA).
// Phase 2 (short): collect data to recenter.
// Phase 3 (short): collect data to rotate the basis.
// Phase 4 (indefinite): full DynPCA algorithm. rotate and add new axis.

const (
	pcaInitDim = 10

	// Phase 2
	phase2Dur = time.Second
	// Phase 3
	phase3Dur     = phase2Dur
	convCritFloor = 0.05 // Floor to apply rotation.

	bucketSensitiveness = 5
)

type dynamicPCA struct {
	// Y = (X-center)' * basis
	centers [mapSize]float64
	basis   *mat.Dense

	sampleN int
	sums    [mapSize]float64
	covMat  *mat.Dense // Covariance Matrix; cumulative
	stats   *basisStats

	// Phase-based initialization
	startT, recenterT      time.Time
	phase2, phase3, phase4 bool
}
type basisStats struct {
	sqNorm   float64    // Square norm (2nd moment); cumulative
	thirdMos *mat.Dense // Third moments; cumulative
	forthMos *mat.Dense // Forth moments; cumulative

	// Histograms (on for each dimensions)
	useHisto bool
	steps    []float64 // The bucket size of each dimension
	histos   []map[int]float64
}

func newDynPCA(queue [][]byte) (ok bool, dynpca *dynamicPCA) {
	dynpca = &dynamicPCA{stats: new(basisStats)}

	// ** 1. Compute centers **
	for _, trace := range queue {
		for j, tr := range trace {
			dynpca.sums[j] += logVals[tr]
		}
	}

	// ** 2. Format Data**
	dynpca.sampleN = len(queue)
	samplesMat := mat.NewDense(dynpca.sampleN, mapSize, nil)
	for j := 0; j < mapSize; j++ {
		dynpca.centers[j] = dynpca.sums[j] / float64(dynpca.sampleN)
		for i := 0; i < dynpca.sampleN; i++ {
			y := logVals[queue[i][j]] - dynpca.centers[j]
			samplesMat.Set(i, j, y)
			dynpca.stats.addSqNorm(y * y)
		}
	}

	// ** 3. PCA **
	var pc stat.PC
	ok = pc.PrincipalComponents(samplesMat, nil)
	if !ok {
		return ok, dynpca
	}
	ok = true

	// ** 4. Prepare Structure **
	vecs := new(mat.Dense)
	pc.VectorsTo(vecs)
	dynpca.basis = mat.DenseCopyOf(vecs.Slice(0, mapSize, 0, pcaInitDim))
	//
	dynpca.covMat = mat.NewDense(pcaInitDim, pcaInitDim, nil)
	vars := pc.VarsTo(nil)
	for i := 0; i < pcaInitDim; i++ {
		dynpca.covMat.Set(i, i, float64(dynpca.sampleN)*vars[i])
	}
	dynpca.stats.initStats(dynpca.basis, samplesMat)
	//
	dynpca.phase2 = true
	dynpca.startT = time.Now()

	return ok, dynpca
}

func (dynpca *dynamicPCA) newSample(trace []byte) {
	if dynpca.phase2 && time.Now().Sub(dynpca.startT) > phase2Dur {
		dynpca.recenter()
		dynpca.recenterT = time.Now()
		dynpca.phase2, dynpca.phase3 = false, true
		//
	} else if dynpca.phase3 && time.Now().Sub(dynpca.recenterT) > phase3Dur {
		// @TODO: start adding new axis (Gram-Schmidt)?
		ok := dynpca.rotate()
		if ok {
			dynpca.phase3, dynpca.phase4 = false, true
		} else {
			dynpca.recenterT = time.Now()
		}
	}

	// ** 1. Center data **
	dynpca.sampleN++
	sampMat := mat.NewDense(1, mapSize, nil)
	for i, tr := range trace {
		v := logVals[tr]
		dynpca.sums[i] += v
		v -= dynpca.centers[i]
		dynpca.stats.addSqNorm(v * v)
		sampMat.Set(0, i, v)
	}

	// ** 2. Project **
	projMat := new(mat.Dense)
	projMat.Mul(sampMat, dynpca.basis)

	// ** 3. Update covariance matrix **
	covs := new(mat.Dense)
	covs.Mul(projMat.T(), projMat)
	dynpca.covMat.Add(dynpca.covMat, covs)
	//
	dynpca.stats.addProj(projMat)
}

func (dynpca *dynamicPCA) recenter() {
	var diff float64
	n := float64(dynpca.sampleN)
	newSampN := dynpca.sampleN / 10
	for i := 0; i < mapSize; i++ {
		c := dynpca.sums[i] / n
		d := dynpca.centers[i] - c
		diff += d * d
		//
		dynpca.centers[i] = c
		dynpca.sums[i] = c * float64(newSampN)
	}
	diff = math.Sqrt(diff)

	m := new(mat.Dense)
	ratio := float64(newSampN) / n
	m.Scale(ratio, dynpca.covMat)
	dynpca.covMat = m
	dynpca.stats.softReset(ratio)
	dynpca.sampleN = newSampN
}

func (dynpca *dynamicPCA) rotate() (ok bool) {
	// ** 1. Prepare data **
	_, basisSize := dynpca.basis.Dims()
	covs := make([]float64, basisSize*basisSize)
	for i := 0; i < basisSize; i++ {
		for j := 0; j < basisSize; j++ {
			cov := dynpca.covMat.At(i, j) / float64(dynpca.sampleN)
			covs[i*basisSize+j] = cov
		}
	}
	covMat := mat.NewSymDense(basisSize, covs)

	// ** 2. Rotate **
	ok, eVals, eVecs := factorize(covMat, basisSize)
	if !ok {
		log.Print("Could not factorize covariance matrix.")
		return ok
	}
	ok = true

	// Test print
	convCrit := computeConvergence(eVecs)
	m := new(mat.Dense)
	m.Scale(1/float64(dynpca.sampleN), dynpca.covMat)
	m.Mul(eVecs.T(), m)
	m.Mul(m, eVecs)

	// ** 3. Apply decomposition **
	if convCrit > convCritFloor {
		dynpca.covMat = mat.NewDense(basisSize, basisSize, nil)
		for i := 0; i < basisSize; i++ {
			dynpca.covMat.Set(i, i, eVals[i]*float64(dynpca.sampleN))
		}
		dynpca.basis.Mul(dynpca.basis, eVecs)
		dynpca.stats.initHisto(eVals)
	}

	return ok
}
func factorize(symMat *mat.SymDense, basisSize int) (
	ok bool, eVals []float64, eVecs *mat.Dense) {
	// Gonum library is a little weird about its eigen decomposiiton
	// implementation. So make a helper function around it.

	var eigsym mat.EigenSym
	ok = eigsym.Factorize(symMat, true)
	if !ok {
		return
	}
	//
	vars := eigsym.Values(nil)
	eVecs = new(mat.Dense)
	eigsym.VectorsTo(eVecs)

	var (
		perm    = make([]int, basisSize)
		permMat = new(mat.Dense)
	)
	eVals = make([]float64, basisSize)
	// a. Find permutation to order eigen vectors/values.
	for i := range perm {
		perm[i] = i
	}
	sort.Slice(perm, func(i, j int) bool {
		indexI, indexJ := perm[i], perm[j]
		return vars[indexI] > vars[indexJ]
	})
	// b. Apply permutation
	for i, index := range perm {
		eVals[i] = vars[index]
	}
	permMat.Permutation(basisSize, perm)
	eVecs.Mul(eVecs, permMat)

	// @TODO: Forth moments should be reset :/
	// But annoying because then I need a number of samples just for this.

	return ok, eVals, eVecs
}
func computeConvergence(ev *mat.Dense) (convCrit float64) {
	r, c := ev.Dims()
	//
	for j := 0; j < c; j++ {
		var maxJ, v float64
		for i := 0; i < r; i++ {
			v = ev.At(i, j)
			v *= v
			if v > maxJ {
				maxJ = v
			}
			convCrit += v
		}
		convCrit -= maxJ
	}
	//
	convCrit /= float64(c)
	return convCrit
}

func (dynpca *dynamicPCA) String() (str string) {
	str = fmt.Sprintf("#sample: %.3v\n", float64(dynpca.sampleN))
	//
	normalizer := 1 / float64(dynpca.sampleN)
	sqNorm := dynpca.stats.sqNorm * normalizer
	//
	var m mat.Dense
	var totSpaceVar float64
	_, basisSize := dynpca.basis.Dims()
	m.Scale(normalizer, dynpca.covMat)
	for i := 0; i < basisSize; i++ {
		totSpaceVar += m.At(i, i)
	}
	str += fmt.Sprintf("Square Norm: %.3v (%.1f%%)\n",
		sqNorm, 100*totSpaceVar/sqNorm)
	//
	tm, fm := dynpca.stats.getMoments(&m, normalizer)
	str += printCompoStats(dynpca.stats, tm, fm)
	//
	str += fmt.Sprintf("Covariance Matrix:\n%.3v", mat.Formatted(&m))

	if false {
		var logDet float64
		for i := 0; i < basisSize; i++ {
			logDet += math.Log(m.At(i, i))
		}
		logDet2, sign := mat.LogDet(&m)
		str += fmt.Sprintf(
			"\nlogDet, logDet2, sign: %.3v, %.3v, %.3v", logDet, logDet2, sign)
	}

	dynpca.recenter()
	return str
}

// *****************************************************************************
// *************************** Basis Statistics ********************************

func (stats *basisStats) initStats(basis, samplesMat *mat.Dense) {
	stats.forthMos = mat.NewDense(1, pcaInitDim, make([]float64, pcaInitDim))
	stats.thirdMos = mat.NewDense(1, pcaInitDim, make([]float64, pcaInitDim))
	projections := new(mat.Dense)
	projections.Mul(samplesMat, basis)
	r, c := projections.Dims()
	for i := 0; i < r; i++ {
		proj := projections.RawRowView(i)
		projMat := mat.NewDense(1, c, proj)
		stats.addProj(projMat)
	}
}

func (stats *basisStats) initHisto(vars []float64) { // This is mostly just allocation.
	stats.useHisto = true
	stats.steps = make([]float64, len(vars))
	stats.histos = make([]map[int]float64, len(vars))
	for i, v := range vars {
		stats.steps[i] = math.Sqrt(v) / bucketSensitiveness
		stats.histos[i] = make(map[int]float64)
		for j := -5; j <= 5; j++ {
			stats.histos[i][j] = 0
		}
	}
}

func (stats *basisStats) addSqNorm(sqNorm float64) {
	stats.sqNorm += sqNorm
}

func tripling(i, j int, v float64) float64    { return v * v * v }
func quadrupling(i, j int, v float64) float64 { return v * v * v * v }
func (stats *basisStats) addProj(projMat *mat.Dense) {
	if stats.useHisto {
		var ok bool
		proj := projMat.RawRowView(0)
		for i, val := range proj {
			// If x \in bucket n, then x \in [n*step, (n+1)*step]
			bucket := int(val / stats.steps[i])
			if val < 0 {
				bucket--
			}
			//
			if _, ok = stats.histos[i][bucket]; !ok {
				stats.histos[i][bucket] = 0
			}
			stats.histos[i][bucket] += 1
		}
	}

	var tripleM, quadM mat.Dense
	tripleM.Apply(tripling, projMat)
	quadM.Apply(quadrupling, projMat)
	stats.thirdMos.Add(stats.thirdMos, &tripleM)
	stats.forthMos.Add(stats.forthMos, &quadM)
}

func (stats *basisStats) softReset(ratio float64) {
	if stats.useHisto {
		for i := range stats.histos {
			for bucket, v := range stats.histos[i] {
				stats.histos[i][bucket] = v * ratio
			}
		}
	}
	stats.sqNorm *= ratio
	stats.forthMos.Scale(ratio, stats.forthMos)
}

func (stats *basisStats) getMoments(covMat *mat.Dense, normalizer float64) (
	tm, fm *mat.Dense) {
	tm, fm = new(mat.Dense), new(mat.Dense)
	tm.Scale(normalizer, stats.thirdMos)
	fm.Scale(normalizer, stats.forthMos)
	tm.Apply(func(i, j int, v float64) float64 {
		variance := covMat.At(j, j)
		return v / math.Pow(variance, 1.5)
	}, tm)
	fm.Apply(func(i, j int, v float64) float64 {
		variance := covMat.At(j, j)
		return v / (variance * variance)
	}, fm)
	return tm, fm
}

// *****************************************************************************
// ************************** Histogram Analysis *******************************
// Some modelization test.

// *** Some distribtutions CDF definition ***
var sqrt2 = math.Sqrt(2)

type distribution interface{ cdf(x float64) float64 }
type uniDist struct{ start, end float64 }
type normalDist struct{ mu, sig float64 }

func (ud uniDist) cdf(x float64) float64 {
	if x < ud.start {
		return 0
	} else if x > ud.end {
		return 1
	}
	return (x - ud.start) / (ud.end - ud.start)
}
func (nd normalDist) cdf(x float64) (c float64) {
	c = (x - nd.mu) / (nd.sig * sqrt2)
	c = 0.5 * (1 + math.Erf(c))
	return c
}

// *** Statistical Test on Histogram ***
func statHistoTest(histo map[int]float64, step float64, dist distribution) (
	test float64) {
	var min, max int
	var tot float64
	for i, v := range histo {
		tot += v
		if i < min {
			min = i
		} else if i > max {
			max = i
		}
	}

	cum := 1.0
	for i := min; i < max; i++ {
		if _, ok := histo[i]; !ok {
			continue
		}
		start, end := float64(i)*step, float64(i+1)*step
		distVal := dist.cdf((start + end) / 2)
		cnt := histo[i]
		test += cramerVonMises(cum, cum+cnt, tot, distVal)
		cum += cnt
	}
	return math.Sqrt(test)
}
func cramerVonMises(start, end, tot, f float64) (test float64) {
	// sum_{i=a}^b ((2.i-1)/(2.n) - f) =
	//   (b-a+1) [(a.a + a.b - 2.a + b.b - b) / (3.n.n) - c.(a+b)/n + c.c]
	squareSum := start*start + start*end - 2*start + end*end - end
	squareSum = squareSum / (3 * tot * tot)
	sum := f * (start + end) / tot
	test = squareSum - sum + f*f
	//
	interval := end - start + 1
	test *= interval / tot
	return test
}

// *** Printing ***
func printCompoStats(stats *basisStats, tm, fm *mat.Dense) string {
	var b strings.Builder
	table := tablewriter.NewWriter(&b)
	table.SetHeader([]string{"PC", "Skewness", "Kurtosis", "Normal_RSS", "Uni RSS"})

	for i, histo := range stats.histos {
		sig := stats.steps[i] * bucketSensitiveness
		normal := normalDist{mu: 0, sig: sig}
		normRSS := statHistoTest(histo, stats.steps[i], normal)
		uniRSS := statHistoTest(histo, stats.steps[i], uniDist{-sig, sig})

		table.Append([]string{
			fmt.Sprintf("%02d", i),
			fmt.Sprintf("%.3v", tm.At(0, i)),
			fmt.Sprintf("%.3v", fm.At(0, i)),
			fmt.Sprintf("%.3v", normRSS),
			fmt.Sprintf("%.3v", uniRSS),
		})
	}

	table.Render()
	return b.String()
}

// *****************************************************************************
// ***************************** Seed Distance *********************************

func klDiv(p, q *dynamicPCA) (div float64) {
	// 1. Project p covariance matric in q basis.
	changeMat, pCovMat := new(mat.Dense), new(mat.Dense)
	changeMat.Mul(p.basis.T(), q.basis)
	//
	pCovMat.Mul(changeMat.T(), p.covMat)
	pCovMat.Mul(pCovMat, changeMat)

	// 2. Compute the divergence
	detP, sp := mat.LogDet(p.covMat)
	detQ, sq := mat.LogDet(q.covMat)
	detP, detQ = detP*sp, detQ*sq
	dim, _ := pCovMat.Dims()
	div = detQ - detP
	fmt.Printf("(step1) div: %.3v\tdetP, detQ: %.3v, %.3v\n", div, detP, detQ)
	//
	inverseQ, prod := new(mat.Dense), new(mat.Dense)
	inverseQ.Inverse(q.covMat)
	prod.Mul(inverseQ, pCovMat)
	tr := prod.Trace() - float64(dim)
	div += tr
	fmt.Printf("(step2) div: %.3v\tTrace-D: %.3v\n", div, tr)
	//
	diff := matDiff(p.centers[:], q.centers[:])
	diffProj, prod2, prod3 := new(mat.Dense), new(mat.Dense), new(mat.Dense)
	diffProj.Mul(diff, q.basis)
	prod2.Mul(diffProj, inverseQ)
	prod3.Mul(prod2, diffProj.T())
	div += prod3.At(0, 0)
	fmt.Printf("(step3) centers dist: %.3v\n", prod3.At(0, 0))
	fmt.Printf("Centers Eucledian distance: %.3v\n",
		euclideanDist(p.centers[:], q.centers[:]))

	div /= 2
	return div
}
func matDiff(mup, muq []float64) *mat.Dense {
	diff := mat.NewDense(1, mapSize, nil)
	for i, tp := range mup {
		tq := muq[i]
		diff.Set(0, i, tq-tp)
	}
	return diff
}
func euclideanDist(mup, muq []float64) (dist float64) {
	for i, tp := range mup {
		diff := tp - muq[i]
		dist += diff * diff
	}
	return math.Sqrt(dist)
}

// *****************************************************************************
// ****************************** Merge Basis **********************************

func mergeBasis(pcas []*dynamicPCA) (glbCenters, vars []float64, glbBasis *mat.Dense) {
	// ** 1. Compute centers **
	glbCenters = make([]float64, mapSize)
	pcaN := float64(len(pcas))
	for i := range glbCenters {
		for _, pca := range pcas {
			glbCenters[i] += pca.centers[i]
		}
		glbCenters[i] /= pcaN
	}

	// ** 2. Prepare PCA **
	var totDim int
	var weights []float64
	for _, pca := range pcas {
		if !pca.phase4 {
			continue
		}
		_, c := pca.basis.Dims()
		totDim += c
		n := float64(pca.sampleN)
		for i := 0; i < c; i++ {
			w := pca.covMat.At(i, i) / n
			weights = append(weights, w)
		}
	}
	//
	var start, end int
	m := mat.NewDense(totDim, mapSize, nil)
	for _, pca := range pcas {
		if !pca.phase4 {
			continue
		}
		_, c := pca.basis.Dims()
		end += c
		w := m.Slice(start, end, 0, mapSize).(*mat.Dense)
		w.Copy(pca.basis.T())
		//
		start = end
	}

	// ** 3. Do PCA **
	var pc stat.PC
	ok := pc.PrincipalComponents(m, weights)
	if !ok {
		log.Print("Couldn't do PCA on basis.")
		return
	}
	//
	vecs := new(mat.Dense)
	pc.VectorsTo(vecs)
	//
	glbBasis = mat.DenseCopyOf(vecs.Slice(0, mapSize, 0, pcaInitDim))
	//
	vars = make([]float64, pcaInitDim)
	for _, pca := range pcas {
		changeBasisM, newCovMat := new(mat.Dense), new(mat.Dense)
		changeBasisM.Mul(pca.basis.T(), glbBasis)
		newCovMat.Mul(changeBasisM.T(), pca.covMat)
		newCovMat.Mul(newCovMat, changeBasisM)
		for i := range vars {
			vars[i] += newCovMat.At(i, i) / float64(pca.sampleN)
		}
	}
	for i := range vars {
		vars[i] /= float64(len(pcas))
	}

	return glbCenters, vars, glbBasis
}

func mahaDist(proj1, proj2, vars []float64) (dist float64) {
	for i, p1 := range proj1 {
		diff := p1 - proj2[i]
		dist += diff * diff / vars[i]
	}
	return math.Sqrt(dist)
}
