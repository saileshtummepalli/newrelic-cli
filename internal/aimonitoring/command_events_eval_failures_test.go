//go:build unit

package aimonitoring

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newrelic/newrelic-client-go/v2/pkg/nrdb"

	"github.com/newrelic/newrelic-cli/internal/client"
	"github.com/newrelic/newrelic-cli/internal/testcobra"
	"github.com/newrelic/newrelic-cli/internal/utils"
)

func TestAIMonitoringEventsEvalFailures(t *testing.T) {
	assert.Equal(t, "failures", cmdEventsEvalFailures.Name())
	testcobra.CheckCobraRequiredFlags(t, cmdEventsEvalFailures, []string{"metric"})
}

func TestBuildFailuresQuery(t *testing.T) {
	got := buildFailuresQuery("JAILBREAK", "7 days ago", 100)

	want := "SELECT evaluatorReason FROM AiEvaluation WHERE (passed = false OR verdict = 'FAILED') AND metricName = 'JAILBREAK' SINCE 7 days ago LIMIT 100"
	assert.Equal(t, want, got)
}

func TestBuildFailuresQuery_EscapesMetric(t *testing.T) {
	got := buildFailuresQuery("JAIL'BREAK", "1 day ago", 50)

	assert.Contains(t, got, `metricName = 'JAIL\'BREAK'`)
}

func TestEventsEvalFailures_RunsQueryAgainstNRDB(t *testing.T) {
	stubApplicationSearchAPI(t, nrqlGUIDsBody([]string{"gpt-4"}), "")

	result, err := client.NRClient.Nrdb.QueryWithContext(utils.SignalCtx, 12345, nrdb.NRQL(
		buildFailuresQuery("JAILBREAK", "7 days ago", 100),
	))
	require.NoError(t, err)
	assert.NotEmpty(t, result.Results)
}
