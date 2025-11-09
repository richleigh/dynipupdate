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

// MockCloudFlareClient implements CloudFlareAPI for testing
type MockCloudFlareClient struct {
	records       map[string][]*CFRecord // key: "name:type", value: list of records
	updateCalled  int
	createCalled  int
	deleteCalled  int
	nextID        int
}

func (m *MockCloudFlareClient) getRecordID(name, recordType string) string {
	key := name + ":" + recordType
	if records, exists := m.records[key]; exists && len(records) > 0 {
		return records[0].ID
	}
	return ""
}

func (m *MockCloudFlareClient) getRecord(name, recordType string) *CFRecord {
	key := name + ":" + recordType
	if records, exists := m.records[key]; exists && len(records) > 0 {
		return records[0]
	}
	return nil
}

func (m *MockCloudFlareClient) getAllRecords(name, recordType string) []CFRecord {
	key := name + ":" + recordType
	var result []CFRecord
	if records, exists := m.records[key]; exists {
		for _, record := range records {
			result = append(result, *record)
		}
	}
	return result
}

func (m *MockCloudFlareClient) createRecord(name, recordType, content string, proxied bool) bool {
	m.createCalled++
	key := name + ":" + recordType

	// Generate unique ID
	m.nextID++
	newRecord := &CFRecord{
		ID:      fmt.Sprintf("test-id-%d", m.nextID),
		Type:    recordType,
		Name:    name,
		Content: content,
	}

	if _, exists := m.records[key]; !exists {
		m.records[key] = []*CFRecord{}
	}
	m.records[key] = append(m.records[key], newRecord)
	return true
}

func (m *MockCloudFlareClient) updateRecord(recordID, name, recordType, content string, proxied bool) bool {
	m.updateCalled++
	key := name + ":" + recordType
	if records, exists := m.records[key]; exists {
		for _, record := range records {
			if record.ID == recordID {
				record.Content = content
				return true
			}
		}
	}
	return true
}

func (m *MockCloudFlareClient) deleteRecord(recordID, name, recordType string) bool {
	m.deleteCalled++
	key := name + ":" + recordType
	if records, exists := m.records[key]; exists {
		// Remove the record with matching ID
		var filtered []*CFRecord
		for _, record := range records {
			if record.ID != recordID {
				filtered = append(filtered, record)
			}
		}
		if len(filtered) == 0 {
			delete(m.records, key)
		} else {
			m.records[key] = filtered
		}
	}
	return true
}

func (m *MockCloudFlareClient) deleteRecordIfExists(name, recordType string) bool {
	recordID := m.getRecordID(name, recordType)
	if recordID != "" {
		return m.deleteRecord(recordID, name, recordType)
	}
	return true
}

func (m *MockCloudFlareClient) upsertRecord(name, recordType, content string, proxied bool) bool {
	record := m.getRecord(name, recordType)
	if record != nil {
		// Record exists - check if content has changed
		if record.Content == content {
			return true
		}
		return m.updateRecord(record.ID, name, recordType, content, proxied)
	}
	return m.createRecord(name, recordType, content, proxied)
}

func (m *MockCloudFlareClient) ensureRecordExists(name, recordType, content string, proxied bool) bool {
	allRecords := m.getAllRecords(name, recordType)

	// Check if a record with this specific content already exists
	for _, record := range allRecords {
		if record.Content == content {
			return true
		}
	}

	// Record with this content doesn't exist - create it
	return m.createRecord(name, recordType, content, proxied)
}

// TestUpsertRecordNoChange verifies that upsertRecord doesn't call update when content is unchanged
func TestUpsertRecordNoChange(t *testing.T) {
	mock := &MockCloudFlareClient{
		records: make(map[string][]*CFRecord),
	}

	// Create initial record
	mock.records["example.com:A"] = []*CFRecord{
		{
			ID:      "test-123",
			Type:    "A",
			Name:    "example.com",
			Content: "192.168.1.1",
		},
	}

	// Call upsert with same content
	result := mock.upsertRecord("example.com", "A", "192.168.1.1", false)

	if !result {
		t.Error("Expected upsertRecord to return true")
	}

	if mock.updateCalled != 0 {
		t.Errorf("Expected updateRecord not to be called, but was called %d times", mock.updateCalled)
	}

	if mock.createCalled != 0 {
		t.Errorf("Expected createRecord not to be called, but was called %d times", mock.createCalled)
	}
}

// TestUpsertRecordContentChanged verifies that upsertRecord DOES call update when content changes
func TestUpsertRecordContentChanged(t *testing.T) {
	mock := &MockCloudFlareClient{
		records: make(map[string][]*CFRecord),
	}

	// Create initial record
	mock.records["example.com:A"] = []*CFRecord{
		{
			ID:      "test-123",
			Type:    "A",
			Name:    "example.com",
			Content: "192.168.1.1",
		},
	}

	// Call upsert with different content
	result := mock.upsertRecord("example.com", "A", "192.168.1.2", false)

	if !result {
		t.Error("Expected upsertRecord to return true")
	}

	if mock.updateCalled != 1 {
		t.Errorf("Expected updateRecord to be called exactly once, but was called %d times", mock.updateCalled)
	}

	if mock.createCalled != 0 {
		t.Errorf("Expected createRecord not to be called, but was called %d times", mock.createCalled)
	}

	// Verify content was actually updated
	record := mock.getRecord("example.com", "A")
	if record == nil {
		t.Fatal("Record should still exist")
	}
	if record.Content != "192.168.1.2" {
		t.Errorf("Expected content to be updated to 192.168.1.2, got %s", record.Content)
	}
}

// TestUpsertRecordCreate verifies that upsertRecord calls create when record doesn't exist
func TestUpsertRecordCreate(t *testing.T) {
	mock := &MockCloudFlareClient{
		records: make(map[string][]*CFRecord),
	}

	// Call upsert for non-existent record
	result := mock.upsertRecord("example.com", "A", "192.168.1.1", false)

	if !result {
		t.Error("Expected upsertRecord to return true")
	}

	if mock.createCalled != 1 {
		t.Errorf("Expected createRecord to be called exactly once, but was called %d times", mock.createCalled)
	}

	if mock.updateCalled != 0 {
		t.Errorf("Expected updateRecord not to be called, but was called %d times", mock.updateCalled)
	}

	// Verify record was created
	record := mock.getRecord("example.com", "A")
	if record == nil {
		t.Fatal("Record should have been created")
	}
	if record.Content != "192.168.1.1" {
		t.Errorf("Expected content to be 192.168.1.1, got %s", record.Content)
	}
}

// TestMultipleInternalIPs verifies that multiple internal IPs can be registered
func TestMultipleInternalIPs(t *testing.T) {
	mock := &MockCloudFlareClient{
		records: make(map[string][]*CFRecord),
	}

	// Create records for multiple IPs
	ips := []string{"192.168.1.10", "10.0.0.5", "172.16.5.20"}
	for _, ip := range ips {
		if !mock.createRecord("internal.example.com", "A", ip, false) {
			t.Fatalf("Failed to create record for %s", ip)
		}
	}

	// Verify all records were created
	allRecords := mock.getAllRecords("internal.example.com", "A")
	if len(allRecords) != 3 {
		t.Errorf("Expected 3 records, got %d", len(allRecords))
	}

	// Verify each IP is present
	recordIPs := make(map[string]bool)
	for _, record := range allRecords {
		recordIPs[record.Content] = true
	}

	for _, ip := range ips {
		if !recordIPs[ip] {
			t.Errorf("Expected to find record for IP %s", ip)
		}
	}
}

// TestStaleRecordCleanup verifies that stale records are properly deleted
func TestStaleRecordCleanup(t *testing.T) {
	mock := &MockCloudFlareClient{
		records: make(map[string][]*CFRecord),
		nextID:  100,
	}

	// Create initial records
	mock.records["internal.example.com:A"] = []*CFRecord{
		{ID: "test-101", Type: "A", Name: "internal.example.com", Content: "192.168.1.10"},
		{ID: "test-102", Type: "A", Name: "internal.example.com", Content: "10.0.0.5"},
		{ID: "test-103", Type: "A", Name: "internal.example.com", Content: "172.16.5.20"},
	}

	// Get existing records
	existingRecords := mock.getAllRecords("internal.example.com", "A")
	if len(existingRecords) != 3 {
		t.Fatalf("Expected 3 initial records, got %d", len(existingRecords))
	}

	// Simulate detected IPs (only 2 IPs detected, one is missing)
	detectedIPs := map[string]bool{
		"192.168.1.10": true,
		"10.0.0.5":     true,
		// 172.16.5.20 is no longer detected (interface went down)
	}

	// Build map of existing IPs
	existingIPs := make(map[string]string) // content -> recordID
	for _, record := range existingRecords {
		existingIPs[record.Content] = record.ID
	}

	// Delete stale records
	deletedCount := 0
	for content, recordID := range existingIPs {
		if !detectedIPs[content] {
			if mock.deleteRecord(recordID, "internal.example.com", "A") {
				deletedCount++
			}
		}
	}

	// Verify stale record was deleted
	if deletedCount != 1 {
		t.Errorf("Expected 1 stale record to be deleted, deleted %d", deletedCount)
	}

	if mock.deleteCalled != 1 {
		t.Errorf("Expected deleteRecord to be called once, but was called %d times", mock.deleteCalled)
	}

	// Verify remaining records
	remainingRecords := mock.getAllRecords("internal.example.com", "A")
	if len(remainingRecords) != 2 {
		t.Errorf("Expected 2 remaining records, got %d", len(remainingRecords))
	}

	// Verify the deleted IP is gone
	for _, record := range remainingRecords {
		if record.Content == "172.16.5.20" {
			t.Error("Expected stale IP 172.16.5.20 to be deleted, but it's still present")
		}
	}

	// Verify the kept IPs are still there
	foundIPs := make(map[string]bool)
	for _, record := range remainingRecords {
		foundIPs[record.Content] = true
	}

	if !foundIPs["192.168.1.10"] {
		t.Error("Expected IP 192.168.1.10 to remain")
	}
	if !foundIPs["10.0.0.5"] {
		t.Error("Expected IP 10.0.0.5 to remain")
	}
}

// TestNoInternalIPsDeletesAll verifies that all records are deleted when no IPs detected
func TestNoInternalIPsDeletesAll(t *testing.T) {
	mock := &MockCloudFlareClient{
		records: make(map[string][]*CFRecord),
	}

	// Create initial records
	mock.records["internal.example.com:A"] = []*CFRecord{
		{ID: "test-201", Type: "A", Name: "internal.example.com", Content: "192.168.1.10"},
		{ID: "test-202", Type: "A", Name: "internal.example.com", Content: "10.0.0.5"},
	}

	// Simulate no IPs detected (all interfaces down)
	existingRecords := mock.getAllRecords("internal.example.com", "A")

	// Delete all records
	for _, record := range existingRecords {
		mock.deleteRecord(record.ID, "internal.example.com", "A")
	}

	// Verify all records were deleted
	remainingRecords := mock.getAllRecords("internal.example.com", "A")
	if len(remainingRecords) != 0 {
		t.Errorf("Expected 0 remaining records, got %d", len(remainingRecords))
	}

	if mock.deleteCalled != 2 {
		t.Errorf("Expected deleteRecord to be called twice, but was called %d times", mock.deleteCalled)
	}
}

// TestCombinedDomainAllIPv4s verifies that combined domain aggregates internal + external IPv4s
func TestCombinedDomainAllIPv4s(t *testing.T) {
	mock := &MockCloudFlareClient{
		records: make(map[string][]*CFRecord),
	}

	// Simulate internal IPs: 192.168.1.10, 10.0.0.5
	// Simulate external IP: 203.0.113.50
	internalIPs := []string{"192.168.1.10", "10.0.0.5"}
	externalIP := "203.0.113.50"

	// Collect all IPv4s (simulating the logic in main())
	var allIPv4s []string
	allIPv4s = append(allIPv4s, internalIPs...)
	allIPv4s = append(allIPv4s, externalIP)

	// Create A records for all IPv4s
	for _, ip := range allIPv4s {
		if !mock.createRecord("combined.example.com", "A", ip, false) {
			t.Fatalf("Failed to create A record for %s", ip)
		}
	}

	// Verify all A records were created
	allRecords := mock.getAllRecords("combined.example.com", "A")
	if len(allRecords) != 3 {
		t.Errorf("Expected 3 A records, got %d", len(allRecords))
	}

	// Verify each IP is present
	recordIPs := make(map[string]bool)
	for _, record := range allRecords {
		recordIPs[record.Content] = true
	}

	expectedIPs := []string{"192.168.1.10", "10.0.0.5", "203.0.113.50"}
	for _, ip := range expectedIPs {
		if !recordIPs[ip] {
			t.Errorf("Expected to find A record for IP %s", ip)
		}
	}
}

// TestCombinedDomainWithIPv6 verifies that combined domain includes both A and AAAA records
func TestCombinedDomainWithIPv6(t *testing.T) {
	mock := &MockCloudFlareClient{
		records: make(map[string][]*CFRecord),
	}

	// Create A records for IPv4s
	mock.createRecord("combined.example.com", "A", "192.168.1.10", false)
	mock.createRecord("combined.example.com", "A", "203.0.113.50", false)

	// Create AAAA record for IPv6
	mock.createRecord("combined.example.com", "AAAA", "2001:db8::1", false)

	// Verify A records
	aRecords := mock.getAllRecords("combined.example.com", "A")
	if len(aRecords) != 2 {
		t.Errorf("Expected 2 A records, got %d", len(aRecords))
	}

	// Verify AAAA record
	aaaaRecords := mock.getAllRecords("combined.example.com", "AAAA")
	if len(aaaaRecords) != 1 {
		t.Errorf("Expected 1 AAAA record, got %d", len(aaaaRecords))
	}

	if aaaaRecords[0].Content != "2001:db8::1" {
		t.Errorf("Expected AAAA record content to be 2001:db8::1, got %s", aaaaRecords[0].Content)
	}
}

// TestCombinedDomainStaleCleanup verifies stale records are cleaned from combined domain
func TestCombinedDomainStaleCleanup(t *testing.T) {
	mock := &MockCloudFlareClient{
		records: make(map[string][]*CFRecord),
	}

	// Create initial records (3 internal IPs + 1 external)
	mock.records["combined.example.com:A"] = []*CFRecord{
		{ID: "test-301", Type: "A", Name: "combined.example.com", Content: "192.168.1.10"},
		{ID: "test-302", Type: "A", Name: "combined.example.com", Content: "10.0.0.5"},
		{ID: "test-303", Type: "A", Name: "combined.example.com", Content: "172.16.5.20"},
		{ID: "test-304", Type: "A", Name: "combined.example.com", Content: "203.0.113.50"}, // external
	}

	// Simulate new state: one internal IP disappeared, external IP changed
	newIPv4s := []string{
		"192.168.1.10", // still present
		"10.0.0.5",     // still present
		// 172.16.5.20 is gone (interface down)
		"203.0.113.99", // external IP changed
	}

	// Get existing records
	existingRecords := mock.getAllRecords("combined.example.com", "A")
	if len(existingRecords) != 4 {
		t.Fatalf("Expected 4 initial records, got %d", len(existingRecords))
	}

	// Build map of existing IPs
	existingIPs := make(map[string]string) // content -> recordID
	for _, record := range existingRecords {
		existingIPs[record.Content] = record.ID
	}

	// Build map of detected IPs
	detectedIPs := make(map[string]bool)
	for _, ip := range newIPv4s {
		detectedIPs[ip] = true
	}

	// Ensure new IPs exist (using ensureRecordExists for multi-record scenario)
	for _, ip := range newIPv4s {
		mock.ensureRecordExists("combined.example.com", "A", ip, false)
	}

	// Delete stale records
	deletedCount := 0
	for content, recordID := range existingIPs {
		if !detectedIPs[content] {
			if mock.deleteRecord(recordID, "combined.example.com", "A") {
				deletedCount++
			}
		}
	}

	// Verify 2 stale records were deleted (172.16.5.20 and old external 203.0.113.50)
	if deletedCount != 2 {
		t.Errorf("Expected 2 stale records to be deleted, deleted %d", deletedCount)
	}

	// Verify remaining records
	remainingRecords := mock.getAllRecords("combined.example.com", "A")
	if len(remainingRecords) != 3 {
		t.Errorf("Expected 3 remaining records, got %d", len(remainingRecords))
	}

	// Verify the correct IPs remain
	foundIPs := make(map[string]bool)
	for _, record := range remainingRecords {
		foundIPs[record.Content] = true
	}

	expectedRemaining := []string{"192.168.1.10", "10.0.0.5", "203.0.113.99"}
	for _, ip := range expectedRemaining {
		if !foundIPs[ip] {
			t.Errorf("Expected IP %s to remain", ip)
		}
	}

	// Verify stale IPs are gone
	if foundIPs["172.16.5.20"] {
		t.Error("Expected stale IP 172.16.5.20 to be deleted")
	}
	if foundIPs["203.0.113.50"] {
		t.Error("Expected old external IP 203.0.113.50 to be deleted")
	}
}

// TestCombinedDomainEmptyIPs verifies all records deleted when no IPs detected
func TestCombinedDomainEmptyIPs(t *testing.T) {
	mock := &MockCloudFlareClient{
		records: make(map[string][]*CFRecord),
	}

	// Create initial records
	mock.records["combined.example.com:A"] = []*CFRecord{
		{ID: "test-401", Type: "A", Name: "combined.example.com", Content: "192.168.1.10"},
		{ID: "test-402", Type: "A", Name: "combined.example.com", Content: "203.0.113.50"},
	}
	mock.records["combined.example.com:AAAA"] = []*CFRecord{
		{ID: "test-403", Type: "AAAA", Name: "combined.example.com", Content: "2001:db8::1"},
	}

	// Simulate no IPs detected (all interfaces down)
	existingARecords := mock.getAllRecords("combined.example.com", "A")
	for _, record := range existingARecords {
		mock.deleteRecord(record.ID, "combined.example.com", "A")
	}

	existingAAAARecords := mock.getAllRecords("combined.example.com", "AAAA")
	for _, record := range existingAAAARecords {
		mock.deleteRecord(record.ID, "combined.example.com", "AAAA")
	}

	// Verify all records were deleted
	remainingARecords := mock.getAllRecords("combined.example.com", "A")
	if len(remainingARecords) != 0 {
		t.Errorf("Expected 0 remaining A records, got %d", len(remainingARecords))
	}

	remainingAAAARecords := mock.getAllRecords("combined.example.com", "AAAA")
	if len(remainingAAAARecords) != 0 {
		t.Errorf("Expected 0 remaining AAAA records, got %d", len(remainingAAAARecords))
	}

	if mock.deleteCalled != 3 {
		t.Errorf("Expected deleteRecord to be called 3 times, but was called %d times", mock.deleteCalled)
	}
}
