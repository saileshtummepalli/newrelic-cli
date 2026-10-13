//go:build unit

package aimonitoring

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/newrelic/newrelic-cli/internal/client"
	"github.com/newrelic/newrelic-client-go/v2/newrelic"
	"github.com/newrelic/newrelic-client-go/v2/pkg/nerdgraph"
	"github.com/newrelic/newrelic-client-go/v2/pkg/testhelpers"
)

// stubNerdGraphSequence points client.NRClient at a test server that answers
// successive NerdGraph requests with each body in order, cycling back to the
// last body once exhausted. It restores the original client.NRClient when
// the test finishes.
func stubNerdGraphSequence(t *testing.T, bodies []string) {
	t.Helper()

	var call int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		body := bodies[call]
		if call < len(bodies)-1 {
			call++
		}

		w.Header().Set("Content-Type", "application/json")
		_, writeErr := w.Write([]byte(body))
		require.NoError(t, writeErr)
	}))
	t.Cleanup(server.Close)

	tc := testhelpers.NewTestConfig(t, server)

	original := client.NRClient
	client.NRClient = &newrelic.NewRelic{
		NerdGraph: nerdgraph.New(tc),
	}
	t.Cleanup(func() { client.NRClient = original })
}

// evaluationsEntityID builds a fake entity ID encoding the given account ID
// and entity type, matching the real "<accountId>|NGEP|<type>|<uuid>" shape.
func evaluationsEntityID(accountID int, entityType string) string {
	return base64.RawStdEncoding.EncodeToString([]byte(fmt.Sprintf("%d|NGEP|%s|test-uuid", accountID, entityType)))
}

func TestAIMonitoringEventsEvalTypes(t *testing.T) {
	assert.Equal(t, "types", cmdEventsEvalTypes.Name())
}

func TestEntityAccountID(t *testing.T) {
	assert.Equal(t, 12345, entityAccountID(evaluationsEntityID(12345, managedEvaluationsEntityType)))
	assert.Equal(t, 0, entityAccountID("not-valid-base64!!"))
}

func entitySearchBody(entities []map[string]interface{}, nextCursor interface{}) string {
	body, _ := json.Marshal(map[string]interface{}{
		"data": map[string]interface{}{
			"actor": map[string]interface{}{
				"entityManagement": map[string]interface{}{
					"entitySearch": map[string]interface{}{
						"nextCursor": nextCursor,
						"entities":   entities,
					},
				},
			},
		},
	})
	return string(body)
}

func TestFetchEvaluationConfigs_FindsMatchingAccount(t *testing.T) {
	stubNerdGraphSequence(t, []string{
		entitySearchBody([]map[string]interface{}{
			{
				"id": evaluationsEntityID(12345, managedEvaluationsEntityType),
				"evaluations": []map[string]interface{}{
					{"evaluationType": "TOXICITY", "threshold": 0.5, "enabled": true},
				},
			},
		}, nil),
	})

	got, err := fetchEvaluationConfigs(managedEvaluationsEntityType, managedEvaluationsFragment, 12345)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, evaluationConfig{EvaluationType: "TOXICITY", Threshold: 0.5, Enabled: true}, got[0])
}

func TestFetchEvaluationConfigs_Paginates(t *testing.T) {
	stubNerdGraphSequence(t, []string{
		entitySearchBody([]map[string]interface{}{
			{"id": evaluationsEntityID(1, managedEvaluationsEntityType), "evaluations": []map[string]interface{}{}},
		}, "cursor-1"),
		entitySearchBody([]map[string]interface{}{
			{
				"id": evaluationsEntityID(12345, managedEvaluationsEntityType),
				"evaluations": []map[string]interface{}{
					{"evaluationType": "JAILBREAK", "threshold": 0.15, "enabled": true},
				},
			},
		}, nil),
	})

	got, err := fetchEvaluationConfigs(managedEvaluationsEntityType, managedEvaluationsFragment, 12345)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "JAILBREAK", got[0].EvaluationType)
}

func TestFetchEvaluationConfigs_NoMatch(t *testing.T) {
	stubNerdGraphSequence(t, []string{
		entitySearchBody([]map[string]interface{}{
			{"id": evaluationsEntityID(1, managedEvaluationsEntityType), "evaluations": []map[string]interface{}{}},
		}, nil),
	})

	got, err := fetchEvaluationConfigs(managedEvaluationsEntityType, managedEvaluationsFragment, 12345)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestFetchEvaluationConfigs_Custom(t *testing.T) {
	stubNerdGraphSequence(t, []string{
		entitySearchBody([]map[string]interface{}{
			{
				"id": evaluationsEntityID(12345, customEvaluationsEntityType),
				"evaluations": []map[string]interface{}{
					{"evaluationType": "CUSTOM_TOXICITY", "threshold": 0.5, "enabled": true},
				},
			},
		}, nil),
	})

	got, err := fetchEvaluationConfigs(customEvaluationsEntityType, customEvaluationsFragment, 12345)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "CUSTOM_TOXICITY", got[0].EvaluationType)
}

func TestEventsEvalTypes_ManagedAndCustomMutuallyExclusive(t *testing.T) {
	err := cmdEventsEvalTypes.Flags().Set("managed", "true")
	require.NoError(t, err)
	err = cmdEventsEvalTypes.Flags().Set("custom", "true")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, cmdEventsEvalTypes.Flags().Set("managed", "false"))
		require.NoError(t, cmdEventsEvalTypes.Flags().Set("custom", "false"))
	})

	assert.Error(t, cmdEventsEvalTypes.ValidateFlagGroups())
}
