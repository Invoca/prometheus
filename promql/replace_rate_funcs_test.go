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
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/promql/parser"
)

func TestReplaceRateFuncs2(t *testing.T) {
	if os.Getenv("PROMQL_TEST_REPLACE_RATE_FUNCS") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestReplaceRateFuncs2$")
		cmd.Env = append(os.Environ(),
			"REPLACE_RATE_FUNCS=2",
			"PROMQL_TEST_REPLACE_RATE_FUNCS=1",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "subprocess failed:\n%s", out)
		return
	}

	require.NotNil(t, parser.Functions["rate"])
	require.NotNil(t, parser.Functions["yrate"])
	require.NotNil(t, parser.Functions["_rate"])
	require.NotNil(t, parser.Functions["xrate"])
	require.Equal(t, "rate", parser.Functions["rate"].Name)
	require.Equal(t, "yrate", parser.Functions["yrate"].Name)
	require.Equal(t, "_rate", parser.Functions["_rate"].Name)
	require.True(t, rateFuncPointersEqual(FunctionCalls["rate"], FunctionCalls["yrate"]))
	require.False(t, rateFuncPointersEqual(FunctionCalls["_rate"], FunctionCalls["rate"]))
	require.False(t, rateFuncPointersEqual(FunctionCalls["rate"], FunctionCalls["xrate"]))
}
