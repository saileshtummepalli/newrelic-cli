package aimonitoring

import (
	"encoding/base64"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/newrelic/newrelic-cli/internal/client"
	configAPI "github.com/newrelic/newrelic-cli/internal/config/api"
	"github.com/newrelic/newrelic-cli/internal/output"
	"github.com/newrelic/newrelic-cli/internal/utils"
)

// managedEvaluationTypeEnumQuery introspects the EntityManagementManagedEvaluationType
// enum, which lists the built-in (non-custom) LLM evaluation types available
// in AI Monitoring.
const managedEvaluationTypeEnumQuery = `query GetManagedEvaluationTypeEnum {
	__type(name: "EntityManagementManagedEvaluationType") {
		name
		kind
		description
		enumValues { name description }
	}
}`

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

// managedEvaluationTypeEnum is the shape of the EntityManagementManagedEvaluationType enum.
type managedEvaluationTypeEnum struct {
	Name        string                           `json:"name"`
	Kind        string                           `json:"kind"`
	Description string                           `json:"description"`
	EnumValues  []managedEvaluationTypeEnumValue `json:"enumValues"`
}

type managedEvaluationTypeEnumValue struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// managedEvaluationConfig is one entry of EntityManagementManagedEvaluationConfig,
// the account's configured threshold/enabled state for a managed evaluation type.
type managedEvaluationConfig struct {
	EvaluationType string  `json:"evaluationType"`
	Threshold      float64 `json:"threshold"`
	Enabled        bool    `json:"enabled"`
}

// managedEvaluationTypeInfo is a managed evaluation type merged with the
// account's configured threshold/enabled state, when available.
type managedEvaluationTypeInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Enabled     *bool    `json:"enabled,omitempty"`
	Threshold   *float64 `json:"threshold,omitempty"`
}

var cmdEventsEvalTypes = &cobra.Command{
	Use:   "types",
	Short: "List the built-in AI Monitoring managed evaluation types",
	Long: `List the built-in AI Monitoring managed evaluation types

The types command lists the enum values of EntityManagementManagedEvaluationType
— the built-in (non-custom) LLM evaluation types available in AI Monitoring —
along with the active account's configured threshold and enabled state for
each type, where configured.
`,
	Example: `newrelic aimonitoring events eval types`,
	PreRun:  client.RequireClient,
	Run: func(cmd *cobra.Command, args []string) {
		enum, err := fetchManagedEvaluationTypes()
		utils.LogIfFatal(err)

		if enum == nil {
			log.Fatal("EntityManagementManagedEvaluationType was not found")
		}

		accountID := configAPI.RequireActiveProfileAccountID()

		configs, err := fetchManagedEvaluationConfigs(accountID)
		utils.LogIfFatal(err)

		configByType := make(map[string]managedEvaluationConfig, len(configs))
		for _, c := range configs {
			configByType[c.EvaluationType] = c
		}

		results := make([]managedEvaluationTypeInfo, 0, len(enum.EnumValues))
		for _, v := range enum.EnumValues {
			info := managedEvaluationTypeInfo{Name: v.Name, Description: v.Description}
			if c, ok := configByType[v.Name]; ok {
				info.Enabled = &c.Enabled
				info.Threshold = &c.Threshold
			}
			results = append(results, info)
		}

		utils.LogIfFatal(output.Print(results))
	},
}

// fetchManagedEvaluationTypes resolves the EntityManagementManagedEvaluationType
// enum via a NerdGraph introspection query.
func fetchManagedEvaluationTypes() (*managedEvaluationTypeEnum, error) {
	var resp struct {
		Type *managedEvaluationTypeEnum `json:"__type"`
	}

	if err := client.NRClient.NerdGraph.QueryWithResponseAndContext(utils.SignalCtx, managedEvaluationTypeEnumQuery, nil, &resp); err != nil {
		return nil, err
	}

	return resp.Type, nil
}

// fetchManagedEvaluationConfigs returns the given account's configured
// threshold/enabled state per managed evaluation type, paging through
// entitySearch until the account's MANAGED_EVALUATIONS entity is found. It
// returns a nil slice, with no error, if the account has no such entity.
func fetchManagedEvaluationConfigs(accountID int) ([]managedEvaluationConfig, error) {
	type entity struct {
		ID          string                    `json:"id"`
		Evaluations []managedEvaluationConfig `json:"evaluations"`
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
