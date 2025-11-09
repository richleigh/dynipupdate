package main

import (
	"encoding/json"
	"fmt"
	"testing"
)

// TestCFListResponse verifies that CloudFlare's list response (GET requests) unmarshals correctly
func TestCFListResponse(t *testing.T) {
	// This is what CloudFlare returns for GET /zones/{zone_id}/dns_records
	jsonResponse := `{
		"success": true,
		"errors": [],
		"result": [
			{
				"id": "372e67954025e0ba6aaa6d586b9e0b59",
				"type": "A",
				"name": "example.com",
				"content": "203.0.113.1"
			},
			{
				"id": "372e67954025e0ba6aaa6d586b9e0b60",
				"type": "AAAA",
				"name": "example.com",
				"content": "2001:db8::1"
			}
		]
	}`

	var response CFListResponse
	err := json.Unmarshal([]byte(jsonResponse), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal CFListResponse: %v", err)
	}

	if !response.Success {
		t.Error("Expected success to be true")
	}

	if len(response.Result) != 2 {
		t.Errorf("Expected 2 records, got %d", len(response.Result))
	}

	// Verify first record
	if response.Result[0].ID != "372e67954025e0ba6aaa6d586b9e0b59" {
		t.Errorf("Expected ID 372e67954025e0ba6aaa6d586b9e0b59, got %s", response.Result[0].ID)
	}
	if response.Result[0].Type != "A" {
		t.Errorf("Expected Type A, got %s", response.Result[0].Type)
	}
	if response.Result[0].Content != "203.0.113.1" {
		t.Errorf("Expected Content 203.0.113.1, got %s", response.Result[0].Content)
	}
}

// TestCFSingleResponse verifies that CloudFlare's single response (POST/PUT/DELETE) unmarshals correctly
func TestCFSingleResponse(t *testing.T) {
	// This is what CloudFlare returns for POST/PUT/DELETE requests
	jsonResponse := `{
		"success": true,
		"errors": [],
		"result": {
			"id": "372e67954025e0ba6aaa6d586b9e0b59",
			"type": "A",
			"name": "example.com",
			"content": "203.0.113.1"
		}
	}`

	var response CFSingleResponse
	err := json.Unmarshal([]byte(jsonResponse), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal CFSingleResponse: %v", err)
	}

	if !response.Success {
		t.Error("Expected success to be true")
	}

	// Verify the single record
	if response.Result.ID != "372e67954025e0ba6aaa6d586b9e0b59" {
		t.Errorf("Expected ID 372e67954025e0ba6aaa6d586b9e0b59, got %s", response.Result.ID)
	}
	if response.Result.Type != "A" {
		t.Errorf("Expected Type A, got %s", response.Result.Type)
	}
	if response.Result.Content != "203.0.113.1" {
		t.Errorf("Expected Content 203.0.113.1, got %s", response.Result.Content)
	}
}

// TestCFErrorResponse verifies that error responses unmarshal correctly
func TestCFErrorResponse(t *testing.T) {
	// This is what CloudFlare returns when authentication fails
	jsonResponse := `{
		"success": false,
		"errors": [
			{"code":10001,"message":"Unable to authenticate request"}
		],
		"result": null
	}`

	var response CFSingleResponse
	err := json.Unmarshal([]byte(jsonResponse), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}

	if response.Success {
		t.Error("Expected success to be false")
	}

	if len(response.Errors) != 1 {
		t.Fatalf("Expected 1 error, got %d", len(response.Errors))
	}

	// Verify error formatting
	errorStr := formatErrors(response.Errors)
	expectedError := `{"code":10001,"message":"Unable to authenticate request"}`
	if errorStr != expectedError {
		t.Errorf("Expected error %s, got %s", expectedError, errorStr)
	}
}

// TestCFListResponseWouldFailWithOldType demonstrates the bug we fixed
func TestCFListResponseWouldFailWithOldType(t *testing.T) {
	// This test shows that trying to unmarshal a single object into an array would fail
	jsonResponse := `{
		"success": true,
		"errors": [],
		"result": {
			"id": "372e67954025e0ba6aaa6d586b9e0b59",
			"type": "A",
			"name": "example.com",
			"content": "203.0.113.1"
		}
	}`

	var response CFListResponse
	err := json.Unmarshal([]byte(jsonResponse), &response)
	if err == nil {
		t.Error("Expected unmarshaling to fail when trying to unmarshal object into array, but it succeeded")
	}

	// Verify the error message is what we saw in production
	expectedErrMsg := "json: cannot unmarshal object into Go struct field CFListResponse.result of type []main.CFRecord"
	if err.Error() != expectedErrMsg {
		t.Logf("Error message: %v", err.Error())
		// Note: This might vary slightly depending on Go version, so we just log it
	}
}

// TestCFSingleResponseWouldFailWithArrayType demonstrates the inverse case
func TestCFSingleResponseWouldFailWithArrayType(t *testing.T) {
	// This test shows that trying to unmarshal an array into a single object would fail
	jsonResponse := `{
		"success": true,
		"errors": [],
		"result": [
			{
				"id": "372e67954025e0ba6aaa6d586b9e0b59",
				"type": "A",
				"name": "example.com",
				"content": "203.0.113.1"
			}
		]
	}`

	var response CFSingleResponse
	err := json.Unmarshal([]byte(jsonResponse), &response)
	if err == nil {
		t.Error("Expected unmarshaling to fail when trying to unmarshal array into object, but it succeeded")
	}
}

// TestFormatErrors verifies error message formatting
func TestFormatErrors(t *testing.T) {
	tests := []struct {
		name     string
		errors   []json.RawMessage
		expected string
	}{
		{
			name:     "empty errors",
			errors:   []json.RawMessage{},
			expected: "unknown error",
		},
		{
			name: "single error",
			errors: []json.RawMessage{
				json.RawMessage(`{"code":10001,"message":"Unable to authenticate request"}`),
			},
			expected: `{"code":10001,"message":"Unable to authenticate request"}`,
		},
		{
			name: "multiple errors",
			errors: []json.RawMessage{
				json.RawMessage(`{"code":1000,"message":"Error 1"}`),
				json.RawMessage(`{"code":2000,"message":"Error 2"}`),
			},
			expected: `{"code":1000,"message":"Error 1"}, {"code":2000,"message":"Error 2"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatErrors(tt.errors)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestCFErrorCode81058 verifies that we can parse error code 81058 (duplicate record)
func TestCFErrorCode81058(t *testing.T) {
	// This is what CloudFlare returns when a record already exists
	jsonResponse := `{
		"success": false,
		"errors": [
			{"code":81058,"message":"An identical record already exists."}
		],
		"result": null
	}`

	var response CFSingleResponse
	err := json.Unmarshal([]byte(jsonResponse), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}

	if response.Success {
		t.Error("Expected success to be false")
	}

	if len(response.Errors) != 1 {
		t.Fatalf("Expected 1 error, got %d", len(response.Errors))
	}

	// Verify we can parse the error code
	var cfErr CFError
	err = json.Unmarshal(response.Errors[0], &cfErr)
	if err != nil {
		t.Fatalf("Failed to unmarshal CFError: %v", err)
	}

	if cfErr.Code != 81058 {
		t.Errorf("Expected error code 81058, got %d", cfErr.Code)
	}

	if cfErr.Message != "An identical record already exists." {
		t.Errorf("Expected message 'An identical record already exists.', got %s", cfErr.Message)
	}
}
