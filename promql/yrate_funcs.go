// Copyright 2015 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package promql

import (
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/util/annotations"
)

// yIncrease is a utility function for yincrease/yrate/ydelta.
// It calculates the increase of the range (allowing for counter resets if isCounter is true),
// taking into account the sample at the end of the previous range (just before rangeStartMsec).
// It returns the result across the range (rangeStartMsec, rangeEndMsec]. The left-open,
// right-closed convention matches the Prometheus 3.x range-selector semantics
// (see prometheus/prometheus#13213) so that a sample whose timestamp lands exactly on
// a range boundary is attributed to the later range, never to both or neither.
// It always extends the preceding sample's value until the next sample. For
// counters without start timestamps it also includes the unwritten origin
// sample value at the start of every time series.
//
// It is additive over adjacent periods, and therefore composable across any
// partitioning of a wider range into contiguous sub-ranges. For adjacent periods
// p0 and p1 ("adjacent" means p0's rangeEndMsec == p1's rangeStartMsec):
//
//	yIncrease(p0) + yIncrease(p1) == yIncrease(p0 + p1)
func yIncrease(points []FPoint, rangeStartMsec, rangeEndMsec int64, isCounter bool, startTimestamps []int64) float64 {
	if len(points) == 0 {
		return 0
	}

	var lastBeforeRange float64
	if isCounter && startTimestamps == nil {
		// Counter without start timestamps:  This implies an unwritten 0 origin at
		// the start of the series, so the first sample's value counts as an increase.
		lastBeforeRange = 0
	} else {
		// Counter with start timestamps, or Gauge:  The first sample's value does not
		// count as an increase. We achieve this by setting lastBeforeRange to the first sample's value.
		lastBeforeRange = points[0].F
	}

	// The points are in time order, so we can just walk the list once and remember the baseline value
	// seen "before" the range and the last value seen "in" range.
	var lastInRange, inRangeResetIncreases float64
	var currentST int64
	var foundInRangeSample bool
	for i := 0; i < len(points) && points[i].T <= rangeEndMsec; i++ { // Only consider points in (rangeStartMsec, rangeEndMsec].
		prevST := currentST
		if i < len(startTimestamps) && startTimestamps[i] != 0 {
			currentST = startTimestamps[i]
		}

		if points[i].T > rangeStartMsec {
			// In range.
			if isCounter && isYCounterReset(startTimestamps, prevST, currentST, points[i].T, rangeStartMsec, points[i].F, lastInRange) {
				// Counter reset: accumulate as if 0 had come before this sample.
				inRangeResetIncreases += lastInRange

				if !foundInRangeSample {
					// This reset was *also* the first in-range sample. Since we just counted
					// the increase above, we assign lastBeforeRange equal so there's no
					// double-counted increase.
					lastBeforeRange = lastInRange
				}
			}
			foundInRangeSample = true
		} else {
			// Out of range.
			lastBeforeRange = points[i].F
		}
		lastInRange = points[i].F
	}

	return lastInRange - lastBeforeRange + inRangeResetIncreases
}

// isYCounterReset reports whether sample (timestamp, value) is a counter reset
// relative to the previous sample. A value drop always counts. When
// startTimestamps is non-nil, an in-window change of currentST also counts.
func isYCounterReset(startTimestamps []int64, prevST, currentST, timestamp, rangeStartMsec int64, value, lastInRange float64) bool {
	return value < lastInRange || // Counter went backwards.
		(startTimestamps != nil && // We have start timestamps.
			currentST != 0 && currentST != prevST && // currentST changed (and is "usable" == non-zero).
			rangeStartMsec < currentST && currentST < timestamp) // In-window.
}

// rangeFromSelectors extracts points, rangeStartMsec, rangeEndMsec, and rangeSeconds
// from the common (Matrix, MatrixSelector) arguments supplied to yincrease/yrate/ydelta.
// The range is (rangeStartMsec, rangeEndMsec]. That is, every sample in range has the property:
// rangeStartMsec < sample.T <= rangeEndMsec.
func rangeFromSelectors(matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) ([]FPoint, int64, int64, float64) {
	ms := args[0].(*parser.MatrixSelector)
	vs := ms.VectorSelector.(*parser.VectorSelector)

	rangeStartMsec := enh.Ts - durationMilliseconds(ms.Range+vs.Offset)
	rangeEndMsec := enh.Ts - durationMilliseconds(vs.Offset)

	points := matrixVals[0].Floats

	return points, rangeStartMsec, rangeEndMsec, ms.Range.Seconds()
}

func funcYdelta(_ []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	points, rangeStartMsec, rangeEndMsec, _ := rangeFromSelectors(matrixVals, args, enh)
	value := yIncrease(points, rangeStartMsec, rangeEndMsec, false, nil)
	return append(enh.Out, Sample{F: value}), nil
}

func funcYincrease(_ []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	points, rangeStartMsec, rangeEndMsec, _ := rangeFromSelectors(matrixVals, args, enh)
	value := yIncrease(points, rangeStartMsec, rangeEndMsec, true, yStartTimestamps(points, enh))
	return append(enh.Out, Sample{F: value}), nil
}

func funcYrate(_ []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	points, rangeStartMsec, rangeEndMsec, rangeSeconds := rangeFromSelectors(matrixVals, args, enh)
	value := yIncrease(points, rangeStartMsec, rangeEndMsec, true, yStartTimestamps(points, enh)) / rangeSeconds
	return append(enh.Out, Sample{F: value}), nil
}

func yStartTimestamps(points []FPoint, enh *EvalNodeHelper) []int64 {
	if enh.StartTimestamps != nil && len(enh.StartTimestamps.Floats) == len(points) {
		startTimestamps := enh.StartTimestamps.Floats
		for _, startTimestamp := range startTimestamps {
			if startTimestamp != 0 {
				return startTimestamps
			}
		}
	}
	return nil
}

func init() {
	FunctionCalls["ydelta"] = funcYdelta
	FunctionCalls["yincrease"] = funcYincrease
	FunctionCalls["yrate"] = funcYrate
	initReplaceRateFuncs()
}
