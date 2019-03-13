package stats

// Based on https://github.com/VividCortex/gohistogram MIT license
// Updated to be json friendly
// Histogram based on https://www.vividcortex.com/blog/2013/07/08/streaming-approximate-histograms/

import (
	"fmt"
	"sort"
)

// Bin holds a float64 value and count
type Bin struct {
	Value float64 `json:"v"`
	Count float64 `json:"c"`
}

type sortByValue []Bin

func (s sortByValue) Len() int {
	return len(s)
}

func (s sortByValue) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func (s sortByValue) Less(i, j int) bool {
	return s[i].Value < s[j].Value
}

// Histogram stores N bins using the streaming approximate histogram approach
// The histogram is not thread safe
type Histogram struct {
	Bins    []Bin  `json:"bins"`
	MaxBins int    `json:"max"`
	Total   uint64 `json:"total"`
}

// NewHistogram returns a new Histogram with a maximum of n bins.
//
// There is no "optimal" bin count, but somewhere between 20 and 80 bins
// should be sufficient.
func NewHistogram(n int) *Histogram {
	return &Histogram{
		Bins:    make([]Bin, 0),
		MaxBins: n,
		Total:   0,
	}
}

// Scale the buckets by s, this is useful for requests or other
// values that may be in large numbers ie nanoseconds
func (h *Histogram) Scale(s float64) {
	for i := range h.Bins {
		h.Bins[i].Value *= s
	}
}

// Add a value to the histogram, creating a bucket if necessary
func (h *Histogram) Add(n float64) {
	defer h.trim()
	h.Total++
	for i := range h.Bins {
		if h.Bins[i].Value == n {
			h.Bins[i].Count++
			return
		}

		if h.Bins[i].Value > n {

			newbin := Bin{Value: n, Count: 1}
			head := append(make([]Bin, 0), h.Bins[0:i]...)

			head = append(head, newbin)
			tail := h.Bins[i:]
			h.Bins = append(head, tail...)
			return
		}
	}

	h.Bins = append(h.Bins, Bin{Count: 1, Value: n})
}

// Quantile returns the value for the bin at the provided quantile
// This is "approximate" in the since that the bin may straddle the quantile value
func (h *Histogram) Quantile(q float64) float64 {
	count := q * float64(h.Total)
	for i := range h.Bins {
		count -= float64(h.Bins[i].Count)

		if count <= 0 {
			return h.Bins[i].Value
		}
	}

	return -1
}

// CDF returns the value of the cumulative distribution function
// at x
func (h *Histogram) CDF(x float64) float64 {
	count := 0.0
	for i := range h.Bins {
		if h.Bins[i].Value <= x {
			count += float64(h.Bins[i].Count)
		}
	}

	return count / float64(h.Total)
}

// Mean returns the sample mean of the distribution
func (h *Histogram) Mean() float64 {
	if h.Total == 0 {
		return 0
	}

	sum := 0.0

	for i := range h.Bins {
		sum += h.Bins[i].Value * h.Bins[i].Count
	}

	return sum / float64(h.Total)
}

// Variance returns the variance of the distribution
func (h *Histogram) Variance() float64 {
	if h.Total == 0 {
		return 0
	}

	sum := 0.0
	mean := h.Mean()

	for i := range h.Bins {
		sum += (h.Bins[i].Count * (h.Bins[i].Value - mean) * (h.Bins[i].Value - mean))
	}

	return sum / float64(h.Total)
}

// Count returns the total number of entries in the histogram
func (h *Histogram) Count() float64 {
	return float64(h.Total)
}

// MergeWith adds all of the bins from another histogram and then combines
func (h *Histogram) MergeWith(other *Histogram) {
	h.Total += other.Total
	h.Bins = append(h.Bins, other.Bins...)
	sort.Sort(sortByValue(h.Bins))
	h.trim()
}

// trim merges adjacent bins to decrease the bin count to the maximum value
func (h *Histogram) trim() {
	for len(h.Bins) > h.MaxBins {
		// Find closest bins in terms of value
		minDelta := 1e99
		minDeltaIndex := 0
		for i := range h.Bins {
			if i == 0 {
				continue
			}

			if delta := h.Bins[i].Value - h.Bins[i-1].Value; delta < minDelta {
				minDelta = delta
				minDeltaIndex = i
			}
		}

		// We need to merge bins minDeltaIndex-1 and minDeltaIndex
		totalCount := h.Bins[minDeltaIndex-1].Count + h.Bins[minDeltaIndex].Count
		mergedbin := Bin{
			Value: (h.Bins[minDeltaIndex-1].Value*
				h.Bins[minDeltaIndex-1].Count +
				h.Bins[minDeltaIndex].Value*
					h.Bins[minDeltaIndex].Count) /
				totalCount, // weighted average
			Count: totalCount, // summed heights
		}
		head := append(make([]Bin, 0), h.Bins[0:minDeltaIndex-1]...)
		tail := append([]Bin{mergedbin}, h.Bins[minDeltaIndex+1:]...)
		h.Bins = append(head, tail...)
	}
}

// String returns a string reprentation of the histogram,
// which is useful for printing to a terminal.
func (h *Histogram) String() string {
	str := fmt.Sprintf("Total Entries: %d\n", h.Total)

	scale := 1.0
	max := 0.0
	for _, b := range h.Bins {
		if b.Count > max {
			max = b.Count
		}
	}

	if max > 75.0 {
		scale = max / 75.0
	}

	for _, b := range h.Bins {
		bar := ""

		barLength := int(b.Count * scale)
		for j := 0; j < barLength; j++ {
			bar += "*"
		}

		str += fmt.Sprintf("%.2f:\t%s\n", b.Value, bar)
	}

	return str
}
