package main

import (
	"fmt"
	"runtime"
	"sync"
)

const SectionSize = 128

type World int64

func (w World) CalcChunk(x_, z_ int32) bool {
	x, z := int64(x_), int64(z_)
	seed := int64(w) +
		x*x*4987142 +
		x*5947611 +
		z*z*4392871 +
		z*389711
	seed ^= 987234911
	r := NewRandom(seed)
	return r.NextInt(10) == 0
}

func (w World) Search(x0, z0, x1, z1 int32, threshold int, mask Mask) []SearchResult {
	mw, mh := mask.Bounds()
	if mw >= SectionSize || mh >= SectionSize {
		panic("Mask bounds exceed section size")
	}

	sectionCh := make(chan *Section, 8)
	resultCh := make(chan []SearchResult, 8)
	wgroup := new(sync.WaitGroup)
	ctx := searchContext{w, threshold, mask, wgroup, sectionCh, resultCh}
	go ctx.sendSections(x0, z0, x1, z1)

	workerCount := runtime.GOMAXPROCS(0)
	wgroup.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go ctx.search()
	}

	var results []SearchResult
	for sectionResults := range resultCh {
		start := len(results)
		results = append(results, sectionResults...)
		for i := start; i < len(results); i++ {
			for j := i; j > 0; j-- {
				if needSwap(results[j-1], results[j]) {
					results[j-1], results[j] = results[j], results[j-1]
				} else {
					break
				}
			}
		}
	}
	return results
}

func needSwap(a, b SearchResult) bool {
	// First sort by count
	if a.Count != b.Count {
		return a.Count < b.Count
	}

	// Then by distance
	ad2 := a.X*a.X + a.Z*a.Z
	bd2 := b.X*b.X + b.Z*b.Z
	if ad2 != bd2 {
		return ad2 > bd2
	}

	// Then by position, purely to break ties so we get consistent ordering
	if a.X != b.X {
		return a.X > b.X
	}
	return a.Z > b.Z
}

type SearchResult struct {
	Count int
	X, Z  int32
}

type searchContext struct {
	world     World
	threshold int
	mask      Mask
	wgroup    *sync.WaitGroup
	sectionCh chan *Section
	resultCh  chan []SearchResult
}

func (ctx searchContext) sendSections(x0, z0, x1, z1 int32) {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if z0 > z1 {
		z0, z1 = z1, z0
	}

	mx, mz := ctx.mask.Bounds()
	shiftX := SectionSize - mx + 1
	shiftZ := SectionSize - mz + 1

	for x := x0; x < x1; x += shiftX {
		for z := z0; z < z1; z += shiftZ {
			ctx.sectionCh <- &Section{X: x, Z: z}
		}
	}
	close(ctx.sectionCh)

	ctx.wgroup.Wait()
	close(ctx.resultCh)
}

func (ctx searchContext) search() {
	for sec := range ctx.sectionCh {
		sec.Compute(ctx.world)
		results := sec.Search(ctx.mask, ctx.threshold)
		if len(results) > 0 {
			ctx.resultCh <- results
		}
	}
	ctx.wgroup.Done()
}

type Section struct {
	X, Z  int32
	Slime [SectionSize * SectionSize]bool
}

func (sec *Section) Compute(world World) {
	for z := int32(0); z < SectionSize; z++ {
		for x := int32(0); x < SectionSize; x++ {
			sec.Set(x, z, world.CalcChunk(sec.X+x, sec.Z+z))
		}
	}
}

func (sec *Section) Search(mask Mask, threshold int) (results []SearchResult) {
	w, h := mask.Bounds()
	offX, offZ := sec.X+w/2, sec.Z+h/2
	x1, z1 := SectionSize-w, SectionSize-h

	for z := int32(0); z < z1; z++ {
		for x := int32(0); x < x1; x++ {
			// TODO: avoid checking the full mask area every time
			//       This can be done by adding the new and subtracting the old chunks
			count := sec.CheckMask(x, z, mask)
			if count >= threshold {
				results = append(results, SearchResult{count, x + offX, z + offZ})
			}
		}
	}
	return results
}

func (sec *Section) CheckMask(x0, z0 int32, mask Mask) (count int) {
	w, h := mask.Bounds()
	for z := int32(0); z < h; z++ {
		for x := int32(0); x < w; x++ {
			if sec.Get(x+x0, z+z0) && mask.Query(x, z) {
				count++
			}
		}
	}
	return count
}

func secIdx(x, z int32) int {
	assert(x < SectionSize, "x out of range")
	assert(z < SectionSize, "z out of range")
	return int(SectionSize*z + x)
}

func (sec *Section) Set(x, z int32, v bool) {
	sec.Slime[secIdx(x, z)] = v
}

func (sec *Section) Get(x, z int32) bool {
	return sec.Slime[secIdx(x, z)]
}

func (sec *Section) Print() {
	for z := int32(0); z < SectionSize; z++ {
		for x := int32(0); x < SectionSize; x++ {
			if x > 0 {
				fmt.Print(" ")
			}
			if sec.Get(x, z) {
				fmt.Print("x")
			} else {
				fmt.Print(" ")
			}
		}
		fmt.Print("\n")
	}
}