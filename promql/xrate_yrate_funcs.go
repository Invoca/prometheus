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
	"fmt"
	"os"
	"reflect"

	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/util/annotations"
)

// preRangeExtrapolation is a utility function for xrate/xincrease/xdelta.
// It calculates the rate (allowing for counter resets if isCounter is true),
// taking into account the last sample before the range start, and returns
// the result as either per-second (if isRate is true) or overall.
//
// Do not confuse with extendedRate(), which implements anchored/smoothed
// selectors in upstream Prometheus 3.x.
func preRangeExtrapolation(matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper, isCounter, isRate bool) (Vector, annotations.Annotations) {
	ms := args[0].(*parser.MatrixSelector)
	vs := ms.VectorSelector.(*parser.VectorSelector)

	var (
		samples    = matrixVals[0]
		rangeStart = enh.Ts - durationMilliseconds(ms.Range+vs.Offset)
		rangeEnd   = enh.Ts - durationMilliseconds(vs.Offset)
	)

	points := samples.Floats
	if len(points) < 2 {
		return enh.Out, nil
	}
	sampledRange := float64(points[len(points)-1].T - points[0].T)
	averageInterval := sampledRange / float64(len(points)-1)

	firstPoint := 0
	// If the point before the range is too far from rangeStart, drop it.
	if float64(rangeStart-points[0].T) > averageInterval {
		if len(points) < 3 {
			return enh.Out, nil
		}
		firstPoint = 1
		sampledRange = float64(points[len(points)-1].T - points[firstPoint].T)
		averageInterval = sampledRange / float64(len(points)-2)
	}

	var (
		counterCorrection float64
		lastValue         float64
	)
	if isCounter {
		for i := firstPoint; i < len(points); i++ {
			sample := points[i]
			if sample.F < lastValue {
				counterCorrection += lastValue
			}
			lastValue = sample.F
		}
	}
	resultValue := points[len(points)-1].F - points[firstPoint].F + counterCorrection

	// Duration between last sample and boundary of range.
	durationToEnd := float64(rangeEnd - points[len(points)-1].T)

	// If the points cover the whole range (i.e. they start just before the
	// range start and end just before the range end) adjust the value from
	// the sampled range to the requested range.
	if points[firstPoint].T <= rangeStart && durationToEnd < averageInterval {
		adjustToRange := float64(durationMilliseconds(ms.Range))
		resultValue *= (adjustToRange / sampledRange)
	}

	if isRate {
		resultValue /= ms.Range.Seconds()
	}

	return append(enh.Out, Sample{F: resultValue}), nil
}

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
func yIncrease(points []FPoint, rangeStartMsec, rangeEndMsec int64, isCounter bool) float64 {
	var lastBeforeRange, lastInRange, inRangeRestartSkew float64

	if !isCounter && len(points) > 0 {
		lastBeforeRange = points[0].F // Gauges don't start at 0.
	}

	// The points are in time order, so we can just walk the list once and remember the last values
	// seen "before" and "in" range. If there are no values in range, we use the last value before range
	// so that the increase is 0.
	for i := 0; i < len(points) && points[i].T <= rangeEndMsec; i++ { // Only consider points in (rangeStartMsec, rangeEndMsec].
		if points[i].T > rangeStartMsec {
			if isCounter && points[i].F < lastInRange { // Counter reset (process restart).
				inRangeRestartSkew += lastInRange
			}
		} else {
			lastBeforeRange = points[i].F
		}
		lastInRange = points[i].F
	}

	return lastInRange - lastBeforeRange + inRangeRestartSkew
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

func funcXdelta(_ []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return preRangeExtrapolation(matrixVals, args, enh, false, false)
}

func funcXrate(_ []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return preRangeExtrapolation(matrixVals, args, enh, true, true)
}

func funcXincrease(_ []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return preRangeExtrapolation(matrixVals, args, enh, true, false)
}

func funcYdelta(_ []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	points, rangeStartMsec, rangeEndMsec, _ := rangeFromSelectors(matrixVals, args, enh)
	value := yIncrease(points, rangeStartMsec, rangeEndMsec, false)
	return append(enh.Out, Sample{F: value}), nil
}

func funcYincrease(_ []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	points, rangeStartMsec, rangeEndMsec, _ := rangeFromSelectors(matrixVals, args, enh)
	value := yIncrease(points, rangeStartMsec, rangeEndMsec, true)
	return append(enh.Out, Sample{F: value}), nil
}

func funcYrate(_ []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	points, rangeStartMsec, rangeEndMsec, rangeSeconds := rangeFromSelectors(matrixVals, args, enh)
	value := yIncrease(points, rangeStartMsec, rangeEndMsec, true) / rangeSeconds
	return append(enh.Out, Sample{F: value}), nil
}

func init() {
	FunctionCalls["xdelta"] = funcXdelta
	FunctionCalls["xincrease"] = funcXincrease
	FunctionCalls["xrate"] = funcXrate
	FunctionCalls["ydelta"] = funcYdelta
	FunctionCalls["yincrease"] = funcYincrease
	FunctionCalls["yrate"] = funcYrate

	// REPLACE_RATE_FUNCS lets operators swap the built-in rate extrapolation
	// functions with the xrate or yrate family at process start, so
	// Grafana auto-completion, Prometheus tooling, Thanos, etc. continue to
	// work against queries that call the standard rate/increase/delta names.
	//
	// Values:
	//   "1"       - replace rate/increase/delta with xrate/xincrease/xdelta
	//               AND remove the x* names (legacy behaviour).
	//   "x", "X"  - point rate/increase/delta at xrate/xincrease/xdelta;
	//               keep the x* names; preserve upstream implementations as
	//               _rate/_increase/_delta.
	//   "2",      - point rate/increase/delta at yrate/yincrease/ydelta;
	//   "y", "Y"    keep the y* (and x*) names; preserve upstream
	//               implementations as _rate/_increase/_delta.
	switch os.Getenv("REPLACE_RATE_FUNCS") {
	case "1":
		FunctionCalls["delta"] = FunctionCalls["xdelta"]
		FunctionCalls["increase"] = FunctionCalls["xincrease"]
		FunctionCalls["rate"] = FunctionCalls["xrate"]
		delete(FunctionCalls, "xdelta")
		delete(FunctionCalls, "xincrease")
		delete(FunctionCalls, "xrate")

		parser.Functions["delta"] = parser.Functions["xdelta"]
		parser.Functions["increase"] = parser.Functions["xincrease"]
		parser.Functions["rate"] = parser.Functions["xrate"]
		parser.Functions["delta"].Name = "delta"
		parser.Functions["increase"].Name = "increase"
		parser.Functions["rate"].Name = "rate"
		delete(parser.Functions, "xdelta")
		delete(parser.Functions, "xincrease")
		delete(parser.Functions, "xrate")
		fmt.Println("Successfully replaced rate & friends with xrate & friends (and removed xrate & friends function keys).")

	case "x", "X":
		replaceStandardRateFuncs("x")
		fmt.Println("Successfully replaced rate/increase/delta with xrate/xincrease/xdelta; originals available as _rate/_increase/_delta; x* names also available.")

	case "2", "y", "Y":
		replaceStandardRateFuncs("y")
		fmt.Println("Successfully replaced rate/increase/delta with yrate/yincrease/ydelta; originals available as _rate/_increase/_delta; y* and x* names also available.")
	}
}

// replaceStandardRateFuncs preserves upstream delta/increase/rate as
// _delta/_increase/_rate and repoints the standard names at the x* or y* family
// (per replacementPrefix).
func replaceStandardRateFuncs(replacementPrefix string) {
	for _, name := range []string{"delta", "increase", "rate"} {
		setParserFunctionFrom("_"+name, name)
		setFunctionCallFrom("_"+name, name)
		replacement := replacementPrefix + name
		setParserFunctionFrom(name, replacement)
		setFunctionCallFrom(name, replacement)
	}
}

// setParserFunctionFrom registers targetName as a copy of sourceName's parser
// metadata, with Name set to targetName.
func setParserFunctionFrom(targetName, sourceName string) {
	result := *parser.Functions[sourceName]
	result.Name = targetName
	parser.Functions[targetName] = &result
}

// setFunctionCallFrom makes targetName dispatch to sourceName's implementation.
func setFunctionCallFrom(targetName, sourceName string) {
	FunctionCalls[targetName] = FunctionCalls[sourceName]
}

// rateFuncPointersEqual compares two FunctionCall implementations by function
// pointer. Used by tests only.
func rateFuncPointersEqual(a, b FunctionCall) bool {
	return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
}
