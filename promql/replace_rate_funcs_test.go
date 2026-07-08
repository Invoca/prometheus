package promql

import (
	"os"
	"os/exec"
	"reflect"
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
	require.Equal(t, reflect.ValueOf(FunctionCalls["rate"]).Pointer(), reflect.ValueOf(FunctionCalls["yrate"]).Pointer())
	require.NotEqual(t, reflect.ValueOf(FunctionCalls["_rate"]).Pointer(), reflect.ValueOf(FunctionCalls["rate"]).Pointer())
	require.NotEqual(t, reflect.ValueOf(FunctionCalls["rate"]).Pointer(), reflect.ValueOf(FunctionCalls["xrate"]).Pointer())
}
