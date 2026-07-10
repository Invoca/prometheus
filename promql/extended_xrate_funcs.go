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

// extendedXRate is a utility function for xrate/xincrease/xdelta.
// It calculates the rate (allowing for counter resets if isCounter is true),
// taking into account the last sample before the range start, and returns
// the result as either per-second (if isRate is true) or overall.
//
// On the 2.55.1 line this was named extendedRate. Renamed here to avoid
// collision with extendedRate() in functions.go, which implements upstream
// Prometheus 3.x anchored/smoothed selectors.
func extendedXRate(matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper, isCounter, isRate bool) (Vector, annotations.Annotations) {
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

func funcXdelta(_ []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return extendedXRate(matrixVals, args, enh, false, false)
}

func funcXrate(_ []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return extendedXRate(matrixVals, args, enh, true, true)
}

func funcXincrease(_ []Vector, matrixVals Matrix, args parser.Expressions, enh *EvalNodeHelper) (Vector, annotations.Annotations) {
	return extendedXRate(matrixVals, args, enh, true, false)
}

func init() {
	FunctionCalls["xdelta"] = funcXdelta
	FunctionCalls["xincrease"] = funcXincrease
	FunctionCalls["xrate"] = funcXrate
}
