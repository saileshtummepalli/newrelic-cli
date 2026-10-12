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

// managedEvaluationsEntityID builds a fake EntityManagementManagedEvaluationsEntity
// ID encoding the given account ID, matching the real "<accountId>|NGEP|<type>|<uuid>" shape.
func managedEvaluationsEntityID(accountID int) string {
	return base64.RawStdEncoding.EncodeToString([]byte(fmt.Sprintf("%d|NGEP|MANAGED_EVALUATIONS|test-uuid", accountID)))
}

// stubNerdGraphAPI points client.NRClient at a test server that always
// answers NerdGraph requests with the given body. It restores the original
// client.NRClient when the test finishes.
func stubNerdGraphAPI(t *testing.T, body string) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		require.NoError(t, err)

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

func TestFetchManagedEvaluationTypes_Found(t *testing.T) {
	body, err := json.Marshal(map[string]interface{}{
		"data": map[string]interface{}{
			"__type": map[string]interface{}{
				"name":        "EntityManagementManagedEvaluationType",
				"kind":        "ENUM",
				"description": "The type of managed evaluation",
				"enumValues": []map[string]interface{}{
					{"name": "TOXICITY", "description": "Toxicity evaluation"},
					{"name": "BIAS", "description": "Bias evaluation"},
				},
			},
		},
	})
	require.NoError(t, err)
	stubNerdGraphAPI(t, string(body))

	got, err := fetchManagedEvaluationTypes()
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "EntityManagementManagedEvaluationType", got.Name)
	require.Len(t, got.EnumValues, 2)
	assert.Equal(t, "TOXICITY", got.EnumValues[0].Name)
}

func TestFetchManagedEvaluationTypes_NotFound(t *testing.T) {
	body, err := json.Marshal(map[string]interface{}{
		"data": map[string]interface{}{
			"__type": nil,
		},
	})
	require.NoError(t, err)
	stubNerdGraphAPI(t, string(body))

	got, err := fetchManagedEvaluationTypes()
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestFetchManagedEvaluationTypes_ErrorPropagates(t *testing.T) {
	body, err := json.Marshal(map[string]interface{}{
		"errors": []map[string]string{{"message": "internal error"}},
	})
	require.NoError(t, err)
	stubNerdGraphAPI(t, string(body))

	got, err := fetchManagedEvaluationTypes()
	assert.Error(t, err)
	assert.Nil(t, got)
}

func TestAIMonitoringEventsEvalTypes(t *testing.T) {
	assert.Equal(t, "types", cmdEventsEvalTypes.Name())
}

func TestEntityAccountID(t *testing.T) {
	assert.Equal(t, 12345, entityAccountID(managedEvaluationsEntityID(12345)))
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

func TestFetchManagedEvaluationConfigs_FindsMatchingAccount(t *testing.T) {
	stubNerdGraphSequence(t, []string{
		entitySearchBody([]map[string]interface{}{
			{
				"id": managedEvaluationsEntityID(12345),
				"evaluations": []map[string]interface{}{
					{"evaluationType": "TOXICITY", "threshold": 0.5, "enabled": true},
				},
			},
		}, nil),
	})

	got, err := fetchManagedEvaluationConfigs(12345)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, managedEvaluationConfig{EvaluationType: "TOXICITY", Threshold: 0.5, Enabled: true}, got[0])
}

func TestFetchManagedEvaluationConfigs_Paginates(t *testing.T) {
	stubNerdGraphSequence(t, []string{
		entitySearchBody([]map[string]interface{}{
			{"id": managedEvaluationsEntityID(1), "evaluations": []map[string]interface{}{}},
		}, "cursor-1"),
		entitySearchBody([]map[string]interface{}{
			{
				"id": managedEvaluationsEntityID(12345),
				"evaluations": []map[string]interface{}{
					{"evaluationType": "JAILBREAK", "threshold": 0.15, "enabled": true},
				},
			},
		}, nil),
	})

	got, err := fetchManagedEvaluationConfigs(12345)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "JAILBREAK", got[0].EvaluationType)
}

func TestFetchManagedEvaluationConfigs_NoMatch(t *testing.T) {
	stubNerdGraphSequence(t, []string{
		entitySearchBody([]map[string]interface{}{
			{"id": managedEvaluationsEntityID(1), "evaluations": []map[string]interface{}{}},
		}, nil),
	})

	got, err := fetchManagedEvaluationConfigs(12345)
	require.NoError(t, err)
	assert.Nil(t, got)
}
