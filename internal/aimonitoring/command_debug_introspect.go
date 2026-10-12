package aimonitoring

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/newrelic/newrelic-cli/internal/client"
	"github.com/newrelic/newrelic-cli/internal/utils"
)

var cmdDebugIntrospect = &cobra.Command{
	Use:     "debug-introspect [typeName]",
	Short:   "temp",
	Long:    "temp: introspect a NerdGraph type's fields and arguments",
	Example: "newrelic aimonitoring debug-introspect EntityManagementManagedEvaluationsEntity",
	Hidden:  true,
	Args:    cobra.ExactArgs(1),
	PreRun:  client.RequireClient,
	Run: func(cmd *cobra.Command, args []string) {
		q := fmt.Sprintf(`query { __type(name: %q) { name kind fields { name args { name type { name kind ofType { name kind } } } type { name kind ofType { name kind ofType { name kind } } } } inputFields { name type { name kind ofType { name kind ofType { name kind } } } } } }`, args[0])
		var resp struct {
			Type json.RawMessage `json:"__type"`
		}
		err := client.NRClient.NerdGraph.QueryWithResponseAndContext(utils.SignalCtx, q, nil, &resp)
		if err != nil {
			fmt.Println("ERR:", err)
			return
		}
		fmt.Println(string(resp.Type))
	},
}

func init() {
	Command.AddCommand(cmdDebugIntrospect)
}
