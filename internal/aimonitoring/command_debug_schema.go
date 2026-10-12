package aimonitoring

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/newrelic/newrelic-cli/internal/client"
	"github.com/newrelic/newrelic-cli/internal/utils"
)

var cmdDebugSchema = &cobra.Command{
	Use:     "debug-schema",
	Short:   "temp",
	Long:    "temp: print the NerdGraph schema's query and mutation root type names",
	Example: "newrelic aimonitoring debug-schema",
	Hidden:  true,
	PreRun:  client.RequireClient,
	Run: func(cmd *cobra.Command, args []string) {
		q := `query { __schema { mutationType { name } queryType { name } } }`
		var resp struct {
			Schema json.RawMessage `json:"__schema"`
		}
		err := client.NRClient.NerdGraph.QueryWithResponseAndContext(utils.SignalCtx, q, nil, &resp)
		if err != nil {
			fmt.Println("ERR:", err)
			return
		}
		fmt.Println(string(resp.Schema))
	},
}

func init() {
	Command.AddCommand(cmdDebugSchema)
}
