package http

import (
	"strings"
	"testing"
)

func TestEvaluationMetricCatalogRoute(t *testing.T) {
	h, application, closeApp := newTestHertzWithApp(t)
	defer closeApp()
	token := issueToken(t, application, "tenant_a")
	response := performJSON(h, "GET", "/v1/evaluation-metrics", "", token)
	if response.Code != 200 || !strings.Contains(response.Body, `"name":"ndcg_at_k"`) || !strings.Contains(response.Body, `"formula":"DCG@K / IDCG@K`) {
		t.Fatalf("catalog status=%d body=%s", response.Code, response.Body)
	}
}
