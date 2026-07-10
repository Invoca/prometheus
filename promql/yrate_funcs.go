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
// It always extends the preceding sample's value until the next sample, including the
// unwritten origin sample value at the start of every time series.
//
// It is additive over adjacent periods, and therefore composable across any
// partitioning of a wider range into contiguous sub-ranges. For adjacent periods
// p0 and p1 ("adjacent" means p0's rangeEndMsec == p1's rangeStartMsec):
//
//	yIncrease(p0) + yIncrease(p1) == yIncrease(p0 + p1)
func yIncrease(points []FPoint, rangeStartMsec, rangeEndMsec int64, isCounter bool, startTimestamps []int64) float64 {
	var lastBeforeRange, lastInRange, inRangeRestartSkew float64
	var currentST int64

	if !isCounter && len(points) > 0 {
		lastBeforeRange = points[0].F // Gauges don't start at 0.
	}

	// The points are in time order, so we can just walk the list once and remember the last values
	// seen "before" and "in" range. If there are no values in range, we use the last value before range
	// so that the increase is 0.
	for i := 0; i < len(points) && points[i].T <= rangeEndMsec; i++ { // Only consider points in (rangeStartMsec, rangeEndMsec].
		prevST := currentST
		if startTimestamps != nil && startTimestamps[i] != 0 {
			currentST = startTimestamps[i]
		}

		if points[i].T > rangeStartMsec {
			if isCounter && (points[i].F < lastInRange || isYCounterStartTimestampReset(prevST, currentST, points[i].T, rangeStartMsec)) { // Counter reset (process restart).
				inRangeRestartSkew += lastInRange
			}
		} else {
			lastBeforeRange = points[i].F
		}
		lastInRange = points[i].F
	}

	return lastInRange - lastBeforeRange + inRangeRestartSkew
}

func isYCounterStartTimestampReset(prevST, currentST, timestamp, rangeStartMsec int64) bool {
	return currentST != 0 && currentST != prevST && rangeStartMsec < currentST && currentST < timestamp
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
	if enh.StartTimestamps == nil || len(enh.StartTimestamps.Floats) != len(points) {
		return nil
	}

	startTimestamps := enh.StartTimestamps.Floats
	for _, startTimestamp := range startTimestamps {
		if startTimestamp != 0 {
			return startTimestamps
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
