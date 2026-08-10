package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Priyanshubhartistm/agent-guardrail/internal/policy"
)

func TestBuildServeHandler(t *testing.T) {
	path := writeTempPolicy(t, validPolicyYAML)
	var auditBuf bytes.Buffer

	handler, err := buildServeHandler(path, &auditBuf, time.Second)
	if err != nil {
		t.Fatalf("buildServeHandler() unexpected error: %v", err)
	}

	body, _ := json.Marshal(policy.ToolCall{SessionID: "s1", Tool: "get_order"})
	req := httptest.NewRequest(http.MethodPost, "/check", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got policy.Decision
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if got.Type != policy.Allow {
		t.Errorf("Decision.Type = %v, want Allow (reason: %s)", got.Type, got.Reason)
	}

	if auditBuf.Len() == 0 {
		t.Error("expected an audit record to have been written")
	}
}

func TestBuildServeHandler_BadPolicyPath(t *testing.T) {
	if _, err := buildServeHandler("does-not-exist.yaml", &bytes.Buffer{}, time.Second); err == nil {
		t.Fatal("buildServeHandler() expected error for missing policy file, got nil")
	}
}

func TestBuildServeHandler_InvalidPolicy(t *testing.T) {
	path := writeTempPolicy(t, invalidPolicyYAML)
	if _, err := buildServeHandler(path, &bytes.Buffer{}, time.Second); err == nil {
		t.Fatal("buildServeHandler() expected error for invalid policy, got nil")
	}
}
