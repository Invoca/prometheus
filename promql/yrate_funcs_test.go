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
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/promql/parser"
)

func TestYIncreaseFirstInRangeReset(t *testing.T) {
	// All cases: last out-of-range sample is 7; the first in-range sample is a reset.
	// Range is (0, 120000].
	const rangeStart, rangeEnd int64 = 0, 120_000

	cases := []struct {
		name            string
		firstInRange    float64
		startTimestamps []int64 // nil => dropdown-only (no ST); non-nil with in-window ST change => ST reset
		want            float64
	}{
		{
			name:            "1_st_reset_7_to_10",
			firstInRange:    10,
			startTimestamps: []int64{-60_000, 30_000},
			want:            10,
		},
		{
			name:            "2_st_reset_7_to_7",
			firstInRange:    7,
			startTimestamps: []int64{-60_000, 30_000},
			want:            7,
		},
		{
			name:            "3_st_reset_7_to_2",
			firstInRange:    2,
			startTimestamps: []int64{-60_000, 30_000},
			want:            2,
		},
		{
			name:            "4_dropdown_reset_7_to_2",
			firstInRange:    2,
			startTimestamps: nil,
			want:            2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			points := []FPoint{
				{T: 0, F: 7},
				{T: 60_000, F: tc.firstInRange},
			}
			got := yIncrease(points, rangeStart, rangeEnd, true, tc.startTimestamps)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestYIncreaseSingleInRangeSTReset(t *testing.T) {
	// A lone in-range sample with an in-window ST reset is not "increase 0 because
	// there's only one point" — the ST justifies counting the full sample from 0.
	got := yIncrease(
		[]FPoint{{T: 60_000, F: 7}},
		0, 120_000,
		true,
		[]int64{30_000},
	)
	require.Equal(t, 7.0, got)
}

func TestFuncYincreaseUsesStartTimestampResetWhenValueIncreases(t *testing.T) {
	got := callYincrease(t,
		[]FPoint{
			{T: 0, F: 100},
			{T: 60_000, F: 150},
			{T: 120_000, F: 200},
		},
		[]int64{-60_000, 30_000, 0},
		2*time.Minute,
		120_000,
	)

	require.Equal(t, 200.0, got)
}

func TestFuncYincreaseCarriesCurrentStartTimestampAcrossZeros(t *testing.T) {
	got := callYincrease(t,
		[]FPoint{
			{T: 0, F: 100},
			{T: 60_000, F: 150},
			{T: 120_000, F: 180},
			{T: 180_000, F: 220},
		},
		[]int64{-60_000, 30_000, 0, 30_000},
		3*time.Minute,
		180_000,
	)

	require.Equal(t, 220.0, got)
}

func TestFuncYincreaseFallsBackToValueResetsForLeadingZeroStartTimestamps(t *testing.T) {
	got := callYincrease(t,
		[]FPoint{
			{T: 0, F: 100},
			{T: 60_000, F: 80},
			{T: 120_000, F: 120},
		},
		[]int64{0, 0, 90_000},
		2*time.Minute,
		120_000,
	)

	require.Equal(t, 200.0, got)
}

func TestFuncYincreaseUsesValueDropWhenStartTimestampsNil(t *testing.T) {
	matrixVals := Matrix{{Floats: []FPoint{
		{T: 0, F: 100},
		{T: 60_000, F: 80},
		{T: 120_000, F: 120},
	}}}
	enh := &EvalNodeHelper{Ts: 120_000}

	got, _ := funcYincrease(nil, matrixVals, yRangeArgs(2*time.Minute), enh)

	require.Len(t, got, 1)
	require.Equal(t, 120.0, got[0].F) // 120 - 100 + 100 (value-drop reset at t=60s)
}

func TestFuncYincreaseNoImpliedZeroOriginWhenStartTimestampsPresent(t *testing.T) {
	// All samples in-range; ST is before rangeStart so it does not justify a
	// fresh start. With startTimestamps present we must not invent a 0 origin,
	// so the first sample is the baseline and the increase is 50.
	got := callYincrease(t,
		[]FPoint{
			{T: 60_000, F: 100},
			{T: 120_000, F: 150},
		},
		[]int64{-60_000, -60_000},
		2*time.Minute,
		120_000,
	)

	require.Equal(t, 50.0, got)
}

func TestFuncYdeltaIgnoresStartTimestamps(t *testing.T) {
	matrixVals := Matrix{{Floats: []FPoint{
		{T: 0, F: 100},
		{T: 60_000, F: 150},
		{T: 120_000, F: 200},
	}}}
	enh := &EvalNodeHelper{
		Ts:              120_000,
		StartTimestamps: &StartTimestamps{Floats: []int64{-60_000, 30_000, 0}},
	}

	got, _ := funcYdelta(nil, matrixVals, yRangeArgs(2*time.Minute), enh)

	require.Len(t, got, 1)
	require.Equal(t, 100.0, got[0].F)
}

func callYincrease(t *testing.T, points []FPoint, startTimestamps []int64, queryRange time.Duration, evalTime int64) float64 {
	t.Helper()

	matrixVals := Matrix{{Floats: points}}
	enh := &EvalNodeHelper{
		Ts:              evalTime,
		StartTimestamps: &StartTimestamps{Floats: startTimestamps},
	}

	got, _ := funcYincrease(nil, matrixVals, yRangeArgs(queryRange), enh)

	require.Len(t, got, 1)
	return got[0].F
}

func yRangeArgs(queryRange time.Duration) parser.Expressions {
	return parser.Expressions{
		&parser.MatrixSelector{
			VectorSelector: &parser.VectorSelector{},
			Range:          queryRange,
		},
	}
}
