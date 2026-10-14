package aimonitoring

import (
	"encoding/base64"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/newrelic/newrelic-cli/internal/client"
	configAPI "github.com/newrelic/newrelic-cli/internal/config/api"
	"github.com/newrelic/newrelic-cli/internal/output"
	"github.com/newrelic/newrelic-cli/internal/utils"
)

// managedEvaluationConfigQuery finds the account's MANAGED_EVALUATIONS entity
// and its per-type threshold/enabled configuration. entitySearch is scoped to
// the whole organization (every account the caller can see), so results are
// filtered client-side by the accountId encoded in each entity's ID.
const managedEvaluationConfigQuery = `query GetManagedEvaluationConfigs($cursor: String) {
	actor {
		entityManagement {
			entitySearch(query: "type = 'MANAGED_EVALUATIONS'", cursor: $cursor) {
				nextCursor
				entities {
					id
					...on EntityManagementManagedEvaluationsEntity {
						evaluations { evaluationType threshold enabled }
					}
				}
			}
		}
	}
}`

// evaluationConfig is one entry of EntityManagementManagedEvaluationConfig,
// the account's configured threshold/enabled state for a managed evaluation type.
type evaluationConfig struct {
	EvaluationType string  `json:"evaluationType"`
	Threshold      float64 `json:"threshold"`
	Enabled        bool    `json:"enabled"`
}

var cmdEventsEvalTypes = &cobra.Command{
	Use:   "types",
	Short: "List the account's configured AI Monitoring managed evaluation types",
	Long: `List the account's configured AI Monitoring managed evaluation types

The types command lists the active account's configured built-in LLM
evaluation types in AI Monitoring, along with each type's configured
threshold and enabled state.
`,
	Example: `newrelic aimonitoring events eval types`,
	PreRun:  client.RequireClient,
	Run: func(cmd *cobra.Command, args []string) {
		accountID := configAPI.RequireActiveProfileAccountID()

		configs, err := fetchManagedEvaluationConfigs(accountID)
		utils.LogIfFatal(err)

		utils.LogIfFatal(output.Print(configs))
	},
}

// fetchManagedEvaluationConfigs returns the given account's configured
// threshold/enabled state per managed evaluation type, paging through
// entitySearch until the account's MANAGED_EVALUATIONS entity is found. It
// returns a nil slice, with no error, if the account has no such entity.
func fetchManagedEvaluationConfigs(accountID int) ([]evaluationConfig, error) {
	type entity struct {
		ID          string             `json:"id"`
		Evaluations []evaluationConfig `json:"evaluations"`
	}

	var cursor *string
	for {
		var resp struct {
			Actor struct {
				EntityManagement struct {
					EntitySearch struct {
						NextCursor *string  `json:"nextCursor"`
						Entities   []entity `json:"entities"`
					} `json:"entitySearch"`
				} `json:"entityManagement"`
			} `json:"actor"`
		}

		variables := map[string]interface{}{"cursor": cursor}
		if err := client.NRClient.NerdGraph.QueryWithResponseAndContext(utils.SignalCtx, managedEvaluationConfigQuery, variables, &resp); err != nil {
			return nil, err
		}

		search := resp.Actor.EntityManagement.EntitySearch
		for _, e := range search.Entities {
			if entityAccountID(e.ID) == accountID {
				return e.Evaluations, nil
			}
		}

		if search.NextCursor == nil {
			return nil, nil
		}
		cursor = search.NextCursor
	}
}

// entityAccountID extracts the account ID from an entity management entity
// ID, which base64-encodes "<accountId>|<domain>|<type>|<uuid>". Returns 0
// if the ID can't be decoded.
func entityAccountID(id string) int {
	decoded, err := base64.RawStdEncoding.DecodeString(id)
	if err != nil {
		return 0
	}

	accountID, err := strconv.Atoi(strings.SplitN(string(decoded), "|", 2)[0])
	if err != nil {
		return 0
	}

	return accountID
}

func init() {
	cmdEventsEval.AddCommand(cmdEventsEvalTypes)
}
