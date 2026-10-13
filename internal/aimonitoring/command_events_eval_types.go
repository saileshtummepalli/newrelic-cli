package aimonitoring

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/newrelic/newrelic-cli/internal/client"
	configAPI "github.com/newrelic/newrelic-cli/internal/config/api"
	"github.com/newrelic/newrelic-cli/internal/output"
	"github.com/newrelic/newrelic-cli/internal/utils"
)

const (
	managedEvaluationsEntityType = "MANAGED_EVALUATIONS"
	managedEvaluationsFragment   = "EntityManagementManagedEvaluationEntity"
	customEvaluationsEntityType  = "CUSTOM_EVALUATIONS"
	customEvaluationsFragment    = "EntityManagementCustomEvaluationEntity"
)

// evaluationConfigQueryTemplate finds the account's managed or custom
// evaluations entity (depending on the entitySearch type and inline fragment
// substituted in) and its per-type threshold/enabled configuration.
// entitySearch is scoped to the whole organization (every account the caller
// can see), so results are filtered client-side by the accountId encoded in
// each entity's ID.
const evaluationConfigQueryTemplate = `query GetEvaluationConfigs($cursor: String) {
	actor {
		entityManagement {
			entitySearch(query: "type = '%s'", cursor: $cursor) {
				nextCursor
				entities {
					id
					...on %s {
						evaluations { evaluationType threshold enabled }
					}
				}
			}
		}
	}
}`

// evaluationConfig is one entry of EntityManagementManagedEvaluationConfig or
// EntityManagementCustomEvaluationConfig, the account's configured
// threshold/enabled state for an evaluation type.
type evaluationConfig struct {
	EvaluationType string  `json:"evaluationType"`
	Threshold      float64 `json:"threshold"`
	Enabled        bool    `json:"enabled"`
}

var evalTypesCustom bool

var cmdEventsEvalTypes = &cobra.Command{
	Use:   "types",
	Short: "List the account's configured AI Monitoring evaluation types",
	Long: `List the account's configured AI Monitoring evaluation types

The types command lists the active account's configured LLM evaluation
types in AI Monitoring, along with each type's configured threshold and
enabled state. Use --managed (the default) for built-in evaluation types,
or --custom for custom evaluation types.
`,
	Example: `newrelic aimonitoring events eval types --managed
newrelic aimonitoring events eval types --custom`,
	PreRun: client.RequireClient,
	Run: func(cmd *cobra.Command, args []string) {
		entitySearchType, fragmentType := managedEvaluationsEntityType, managedEvaluationsFragment
		if evalTypesCustom {
			entitySearchType, fragmentType = customEvaluationsEntityType, customEvaluationsFragment
		}

		accountID := configAPI.RequireActiveProfileAccountID()

		configs, err := fetchEvaluationConfigs(entitySearchType, fragmentType, accountID)
		utils.LogIfFatal(err)

		utils.LogIfFatal(output.Print(configs))
	},
}

// fetchEvaluationConfigs returns the given account's configured
// threshold/enabled state per evaluation type, paging through entitySearch
// until the account's evaluations entity is found. It returns a nil slice,
// with no error, if the account has no such entity.
func fetchEvaluationConfigs(entitySearchType string, fragmentType string, accountID int) ([]evaluationConfig, error) {
	type entity struct {
		ID          string             `json:"id"`
		Evaluations []evaluationConfig `json:"evaluations"`
	}

	query := fmt.Sprintf(evaluationConfigQueryTemplate, entitySearchType, fragmentType)

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
		if err := client.NRClient.NerdGraph.QueryWithResponseAndContext(utils.SignalCtx, query, variables, &resp); err != nil {
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

	var evalTypesManaged bool
	cmdEventsEvalTypes.Flags().BoolVar(&evalTypesManaged, "managed", false, "list configured built-in (managed) AI Monitoring evaluation types (default)")
	cmdEventsEvalTypes.Flags().BoolVar(&evalTypesCustom, "custom", false, "list configured custom AI Monitoring evaluation types")
	cmdEventsEvalTypes.MarkFlagsMutuallyExclusive("managed", "custom")
}
