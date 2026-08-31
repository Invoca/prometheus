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
)

// initReplaceRateFuncs swaps built-in rate/increase/delta for the x* or y* family
// when REPLACE_RATE_FUNCS is set. Called from yrate_funcs init after x* and y*
// FunctionCalls are registered.
func initReplaceRateFuncs() {
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
