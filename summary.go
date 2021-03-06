package tdigest

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
)

type centroid struct {
	mean  float64
	count uint64
	index int
}

func (c centroid) isValid() bool {
	return !math.IsNaN(c.mean) && c.count > 0
}

func (c *centroid) Update(x float64, weight uint64) {
	c.count += weight
	c.mean += float64(weight) * (x - c.mean) / float64(c.count)
}

var invalidCentroid = centroid{mean: math.NaN(), count: 0}

type summary struct {
	keys   []float64
	counts []uint64
}

func newSummary(initialCapacity uint) *summary {
	return &summary{
		keys:   make([]float64, 0, initialCapacity),
		counts: make([]uint64, 0, initialCapacity),
	}
}

func (s summary) Len() int {
	return len(s.keys)
}

func (s *summary) Add(key float64, value uint64) error {

	if math.IsNaN(key) {
		return fmt.Errorf("Key must not be NaN")
	}

	if value == 0 {
		return fmt.Errorf("Count must be >0")
	}

	idx := s.FindIndex(key)

	if s.meanAtIndexIs(idx, key) {
		s.updateAt(idx, key, value)
		return nil
	}

	s.keys = append(s.keys, math.NaN())
	s.counts = append(s.counts, 0)

	copy(s.keys[idx+1:], s.keys[idx:])
	copy(s.counts[idx+1:], s.counts[idx:])

	s.keys[idx] = key
	s.counts[idx] = value

	return nil
}

func (s summary) Find(x float64) centroid {
	idx := s.FindIndex(x)

	if idx < s.Len() && s.keys[idx] == x {
		return centroid{x, s.counts[idx], idx}
	}

	return invalidCentroid
}

func (s summary) FindIndex(x float64) int {
	// Binary search is only worthwhile if we have a lot of keys.
	if len(s.keys) < 250 {
		for i, item := range s.keys {
			if item >= x {
				return i
			}
		}
		return len(s.keys)
	}

	return sort.SearchFloat64s(s.keys, x)
}

func (s summary) At(index int) centroid {
	if s.Len()-1 < index || index < 0 {
		return invalidCentroid
	}

	return centroid{s.keys[index], s.counts[index], index}
}

func (s summary) Iterate(f func(c centroid) bool) {
	for i := 0; i < s.Len(); i++ {
		if !f(centroid{s.keys[i], s.counts[i], i}) {
			break
		}
	}
}

func (s summary) Min() centroid {
	return s.At(0)
}

func (s summary) Max() centroid {
	return s.At(s.Len() - 1)
}

func (s summary) successorAndPredecessorItems(mean float64) (centroid, centroid) {
	idx := s.FindIndex(mean)
	return s.At(idx + 1), s.At(idx - 1)
}

func (s summary) ceilingAndFloorItems(mean float64) (centroid, centroid) {
	idx := s.FindIndex(mean)

	// Case 1: item is greater than all items in the summary
	if idx == s.Len() {
		return invalidCentroid, s.Max()
	}

	item := s.At(idx)

	// Case 2: item exists in the summary
	if item.isValid() && mean == item.mean {
		return item, item
	}

	// Case 3: item is smaller than all items in the summary
	if idx == 0 {
		return s.Min(), invalidCentroid
	}

	return item, s.At(idx - 1)
}

// This method is the hotspot when calling Add(), which in turn is called by
// Compress() and Merge(). A simple loop unroll saves a surprising amount of
// time.
func (s summary) sumUntilIndex(idx int) uint64 {
	var cumSum uint64
	var i int
	for i = idx - 1; i >= 3; i -= 4 {
		cumSum += s.counts[i]
		cumSum += s.counts[i-1]
		cumSum += s.counts[i-2]
		cumSum += s.counts[i-3]
	}
	for ; i >= 0; i-- {
		cumSum += s.counts[i]
	}

	return cumSum
}

func (s *summary) updateAt(index int, mean float64, count uint64) {
	c := centroid{s.keys[index], s.counts[index], index}
	c.Update(mean, count)

	oldMean := s.keys[index]
	s.keys[index] = c.mean
	s.counts[index] = c.count

	if c.mean > oldMean {
		s.adjustRight(index)
	} else if c.mean < oldMean {
		s.adjustLeft(index)
	}
}

func (s *summary) adjustRight(index int) {
	for i := index + 1; i < len(s.keys) && s.keys[i-1] > s.keys[i]; i++ {
		s.keys[i-1], s.keys[i] = s.keys[i], s.keys[i-1]
		s.counts[i-1], s.counts[i] = s.counts[i], s.counts[i-1]
	}
}

func (s *summary) adjustLeft(index int) {
	for i := index - 1; i >= 0 && s.keys[i] > s.keys[i+1]; i-- {
		s.keys[i], s.keys[i+1] = s.keys[i+1], s.keys[i]
		s.counts[i], s.counts[i+1] = s.counts[i+1], s.counts[i]
	}
}

func (s summary) meanAtIndexIs(index int, mean float64) bool {
	return index < len(s.keys) && s.keys[index] == mean
}

// Randomly shuffles summary contents, so they can be added to another summary
// with being pathological. Renders summary invalid.
func (s *summary) shuffle() {
	for i := len(s.keys) - 1; i > 1; i-- {
		s.Swap(i, rand.Intn(i+1))
	}
}

// Re-sorts summary, repairing the damage done by shuffle().
func (s *summary) unshuffle() {
	sort.Sort(s)
}

// for sort.Interface
func (s *summary) Swap(i, j int) {
	s.keys[i], s.keys[j] = s.keys[j], s.keys[i]
	s.counts[i], s.counts[j] = s.counts[j], s.counts[i]
}

func (s *summary) Less(i, j int) bool {
	return s.keys[i] < s.keys[j]
}
