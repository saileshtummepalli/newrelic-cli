package aimonitoring

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/newrelic/newrelic-client-go/v2/pkg/nrdb"

	"github.com/newrelic/newrelic-cli/internal/client"
	configAPI "github.com/newrelic/newrelic-cli/internal/config/api"
	"github.com/newrelic/newrelic-cli/internal/output"
	"github.com/newrelic/newrelic-cli/internal/utils"
)

var (
	failuresMetric string
	failuresSince  string
	failuresLimit  int
)

var cmdEventsEvalFailures = &cobra.Command{
	Use:   "failures",
	Short: "Find evaluator reasons for failures of an AI Monitoring evaluation metric",
	Long: `Find evaluator reasons for failures of an AI Monitoring evaluation metric

The failures command runs a NRQL query against AiEvaluation events to find
the evaluatorReason values recorded for failures of a given evaluation
metric — for example JAILBREAK. Run "events eval types" to see the built-in
metric names.
`,
	Example: `newrelic aimonitoring events eval failures --metric JAILBREAK --since "7 days ago"`,
	PreRun:  client.RequireClient,
	Run: func(cmd *cobra.Command, args []string) {
		accountID := configAPI.RequireActiveProfileAccountID()

		query := buildFailuresQuery(failuresMetric, failuresSince, failuresLimit)

		result, err := client.NRClient.Nrdb.QueryWithContext(utils.SignalCtx, accountID, nrdb.NRQL(query))
		utils.LogIfFatal(err)

		utils.LogIfFatal(output.Print(result.Results))
	},
}

// buildFailuresQuery builds the NRQL query that finds the evaluatorReason
// values recorded for failures of the given evaluation metric.
func buildFailuresQuery(metric string, since string, limit int) string {
	return fmt.Sprintf(
		"SELECT evaluatorReason FROM AiEvaluation WHERE (passed = false OR verdict = 'FAILED') AND metricName = '%s' SINCE %s LIMIT %d",
		escapeNRQLStringLiteral(metric), since, limit,
	)
}

func init() {
	cmdEventsEval.AddCommand(cmdEventsEvalFailures)

	cmdEventsEvalFailures.Flags().StringVarP(&failuresMetric, "metric", "m", "", "the evaluation metricName to filter failures by (e.g. JAILBREAK)")
	cmdEventsEvalFailures.Flags().StringVar(&failuresSince, "since", "7 days ago", "the NRQL SINCE clause to use")
	cmdEventsEvalFailures.Flags().IntVarP(&failuresLimit, "limit", "l", 100, "the maximum number of events to return (NRQL LIMIT)")

	utils.LogIfError(cmdEventsEvalFailures.MarkFlagRequired("metric"))
}
