package aimonitoring

import (
	"github.com/spf13/cobra"
)

var cmdEventsEval = &cobra.Command{
	Use:     "eval",
	Short:   "Interact with AI Monitoring evaluations",
	Long:    "Interact with AI Monitoring evaluations",
	Example: "newrelic aimonitoring events eval --help",
}

func init() {
	cmdEvents.AddCommand(cmdEventsEval)
}
