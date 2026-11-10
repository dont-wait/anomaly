package openapi

import (
	"reflect"
	"testing"

	"github.com/dont-wait/anomaly/internal/application/reporting"
	"github.com/dont-wait/anomaly/internal/presentation/http/httpx"
)

func TestReportResponseDocumentsSingleContract(t *testing.T) {
	response := responseObject("Transaction report", httpx.SuccessResponse[reporting.Report]{}, "application/json")
	content := response["content"].(map[string]any)["application/json"].(map[string]any)
	schema := content["schema"].(map[string]any)
	if schema["oneOf"] != nil {
		t.Fatalf("report must have a single response schema: %v", schema)
	}
	properties := schema["properties"].(map[string]any)
	data := properties["data"].(map[string]any)["properties"].(map[string]any)
	summary := data["summary"].(map[string]any)["properties"].(map[string]any)
	created := summary["created"].(map[string]any)["properties"].(map[string]any)
	if created["successCount"] == nil || created["transactionCount"] == nil || created["StatusCounts"] != nil {
		t.Fatalf("embedded status counts not flattened like JSON: %v", created)
	}
	risk := summary["risk"].(map[string]any)["properties"].(map[string]any)
	if risk["averageScore"].(map[string]any)["nullable"] != true {
		t.Fatal("unassessed averages must be documented as nullable")
	}
}

func TestReportStatusCountSchemaMatchesJSONFields(t *testing.T) {
	schema := schemaFor(reflect.TypeOf(reporting.CreatedSummary{}))
	properties := schema["properties"].(map[string]any)
	if len(properties) != 7 || len(schema["required"].([]string)) != 7 {
		t.Fatalf("created counts schema = %v", schema)
	}
}
