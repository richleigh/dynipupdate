package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// CloudFlare API structures
type CFListResponse struct {
	Success bool              `json:"success"`
	Errors  []json.RawMessage `json:"errors"`
	Result  []CFRecord        `json:"result"`
}

type CFSingleResponse struct {
	Success bool              `json:"success"`
	Errors  []json.RawMessage `json:"errors"`
	Result  CFRecord          `json:"result"`
}

type CFRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

// Config holds cleanup service configuration
type Config struct {
	CFAPIToken      string
	CFZoneID        string
	InternalDomain  string
	ExternalDomain  string
	IPv6Domain      string
	CombinedDomain  string
	HeartbeatDomain string // Domain where heartbeats are stored (defaults to InternalDomain)
	StaleThreshold  int    // seconds
	CleanupInterval int    // seconds
}

// CloudFlareClient handles CloudFlare API interactions
type CloudFlareClient struct {
	APIToken string
	ZoneID   string
	BaseURL  string
}

func main() {
	log.SetFlags(log.LstdFlags)
	log.Println("Starting DNS Cleanup Service")

	config := loadConfig()

	cf := &CloudFlareClient{
		APIToken: config.CFAPIToken,
		ZoneID:   config.CFZoneID,
		BaseURL:  "https://api.cloudflare.com/client/v4",
	}

	// Run cleanup immediately on startup
	runCleanup(cf, config)

	// Then run periodically
	ticker := time.NewTicker(time.Duration(config.CleanupInterval) * time.Second)
	defer ticker.Stop()

	log.Printf("Cleanup service running. Will check every %d seconds for records older than %d seconds",
		config.CleanupInterval, config.StaleThreshold)

	for range ticker.C {
		runCleanup(cf, config)
	}
}

func runCleanup(cf *CloudFlareClient, config *Config) {
	log.Println("Running cleanup cycle...")

	totalDeleted := 0

	// Cleanup internal domain (A records only) if configured
	if config.InternalDomain != "" {
		deleted := cleanupDomain(cf, config.InternalDomain, "A", config.HeartbeatDomain, config.StaleThreshold)
		totalDeleted += deleted
		log.Printf("Deleted %d stale A records from %s", deleted, config.InternalDomain)
	}

	// Cleanup external domain (A records only) if configured
	if config.ExternalDomain != "" {
		deleted := cleanupDomain(cf, config.ExternalDomain, "A", config.HeartbeatDomain, config.StaleThreshold)
		totalDeleted += deleted
		log.Printf("Deleted %d stale A records from %s", deleted, config.ExternalDomain)
	}

	// Cleanup IPv6 domain (AAAA records only) if configured
	if config.IPv6Domain != "" {
		deleted := cleanupDomain(cf, config.IPv6Domain, "AAAA", config.HeartbeatDomain, config.StaleThreshold)
		totalDeleted += deleted
		log.Printf("Deleted %d stale AAAA records from %s", deleted, config.IPv6Domain)
	}

	// Cleanup combined domain (both A and AAAA records) if configured
	if config.CombinedDomain != "" {
		deletedA := cleanupDomain(cf, config.CombinedDomain, "A", config.HeartbeatDomain, config.StaleThreshold)
		deletedAAAA := cleanupDomain(cf, config.CombinedDomain, "AAAA", config.HeartbeatDomain, config.StaleThreshold)
		totalDeleted += deletedA + deletedAAAA
		log.Printf("Deleted %d stale A and %d stale AAAA records from %s", deletedA, deletedAAAA, config.CombinedDomain)
	}

	log.Printf("Cleanup cycle complete. Total deleted: %d", totalDeleted)
}

func cleanupDomain(cf *CloudFlareClient, domain string, recordType string, heartbeatDomain string, staleThresholdSeconds int) int {
	deletedCount := 0

	// Get all records of the specified type for this domain (will include service subdomains)
	records := cf.getAllRecords(domain, recordType)

	// Group records by instance ID
	recordsByInstance := make(map[string][]CFRecord)
	for _, record := range records {
		// Extract instance ID from record name (e.g., "web-prod-1.internal.example.com" -> "web-prod-1")
		instanceID := extractInstanceID(record.Name, domain)
		if instanceID == "" {
			// This is a direct record on the base domain, not a service subdomain - skip cleanup
			log.Printf("Skipping non-service record: %s", record.Name)
			continue
		}

		if recordsByInstance[instanceID] == nil {
			recordsByInstance[instanceID] = []CFRecord{}
		}
		recordsByInstance[instanceID] = append(recordsByInstance[instanceID], record)
	}

	// Check heartbeat for each instance and delete stale records
	for instanceID, instanceRecords := range recordsByInstance {
		// Get the heartbeat TXT record for this instance
		heartbeatName := heartbeatRecordName(instanceID, heartbeatDomain)
		heartbeatRecords := cf.getAllRecords(heartbeatName, "TXT")

		shouldDelete := false
		deleteReason := ""

		if len(heartbeatRecords) == 0 {
			// No heartbeat record - this service is stale
			shouldDelete = true
			deleteReason = "no heartbeat found"
		} else {
			// Parse the heartbeat content: "timestamp,instanceID"
			heartbeatContent := heartbeatRecords[0].Content
			// Remove quotes if present (CloudFlare returns TXT records with quotes)
			heartbeatContent = strings.Trim(heartbeatContent, "\"")

			parts := strings.Split(heartbeatContent, ",")
			if len(parts) < 1 {
				log.Printf("Invalid heartbeat format for instance %s: %s", instanceID, heartbeatContent)
				shouldDelete = true
				deleteReason = "invalid heartbeat format"
			} else {
				timestamp, err := strconv.ParseInt(parts[0], 10, 64)
				if err != nil {
					log.Printf("Invalid timestamp in heartbeat for instance %s: %s", instanceID, parts[0])
					shouldDelete = true
					deleteReason = "invalid timestamp"
				} else {
					// Check if heartbeat is stale
					age := time.Now().Unix() - timestamp
					if age > int64(staleThresholdSeconds) {
						shouldDelete = true
						deleteReason = fmt.Sprintf("stale heartbeat (age: %ds)", age)
					}
				}
			}
		}

		if shouldDelete {
			log.Printf("Deleting service %s (%s, %d %s records)", instanceID, deleteReason, len(instanceRecords), recordType)

			// Delete all records for this instance
			for _, record := range instanceRecords {
				if cf.deleteRecord(record.ID, record.Name, recordType) {
					deletedCount++
					log.Printf("  Deleted %s record: %s -> %s", recordType, record.Name, record.Content)
				}
			}
		}

			// Delete the heartbeat TXT record
			if len(heartbeatRecords) > 0 {
				cf.deleteRecord(heartbeatRecords[0].ID, heartbeatName, "TXT")
				log.Printf("  Deleted heartbeat: %s", heartbeatName)
			}
		}
	}

	return deletedCount
}

func loadConfig() *Config {
	apiToken := getEnvOrExit("CF_API_TOKEN")
	apiToken = strings.TrimSpace(apiToken)

	internalDomain := os.Getenv("INTERNAL_DOMAIN")
	heartbeatDomain := os.Getenv("HEARTBEAT_DOMAIN")
	if heartbeatDomain == "" {
		heartbeatDomain = internalDomain // Default to internal domain
	}

	config := &Config{
		CFAPIToken:      apiToken,
		CFZoneID:        getEnvOrExit("CF_ZONE_ID"),
		InternalDomain:  internalDomain,
		ExternalDomain:  os.Getenv("EXTERNAL_DOMAIN"),
		IPv6Domain:      os.Getenv("IPV6_DOMAIN"),
		CombinedDomain:  os.Getenv("COMBINED_DOMAIN"),
		HeartbeatDomain: heartbeatDomain,
		StaleThreshold:  getEnvOrDefaultInt("STALE_THRESHOLD_SECONDS", 3600), // 1 hour
		CleanupInterval: getEnvOrDefaultInt("CLEANUP_INTERVAL_SECONDS", 300), // 5 minutes
	}

	log.Printf("Configuration:")
	log.Printf("  Internal Domain: %s", config.InternalDomain)
	log.Printf("  External Domain: %s", config.ExternalDomain)
	log.Printf("  IPv6 Domain: %s", config.IPv6Domain)
	log.Printf("  Combined Domain: %s", config.CombinedDomain)
	log.Printf("  Heartbeat Domain: %s", config.HeartbeatDomain)
	log.Printf("  Stale Threshold: %d seconds", config.StaleThreshold)
	log.Printf("  Cleanup Interval: %d seconds", config.CleanupInterval)

	return config
}

func getEnvOrExit(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("Required environment variable %s not set", key)
	}
	return value
}

func getEnvOrDefaultInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	intValue, err := strconv.Atoi(value)
	if err != nil {
		log.Printf("Invalid integer value for %s: %s, using default %d", key, value, defaultValue)
		return defaultValue
	}
	return intValue
}

// extractInstanceID extracts the instance ID from a full DNS record name
// Example: "web-prod-1.internal.example.com", "internal.example.com" -> "web-prod-1"
func extractInstanceID(recordName, baseDomain string) string {
	// Remove trailing dot if present
	recordName = strings.TrimSuffix(recordName, ".")
	baseDomain = strings.TrimSuffix(baseDomain, ".")

	// Record name should be: instanceID.baseDomain
	suffix := "." + baseDomain
	if !strings.HasSuffix(recordName, suffix) {
		return "" // Not a service subdomain
	}

	instanceID := strings.TrimSuffix(recordName, suffix)
	return instanceID
}

// heartbeatRecordName creates the TXT record name for a service heartbeat
// Example: "web-prod-1", "internal.example.com" -> "_heartbeat.web-prod-1.internal.example.com"
func heartbeatRecordName(instanceID, baseDomain string) string {
	if baseDomain == "" {
		return ""
	}
	return fmt.Sprintf("_heartbeat.%s.%s", instanceID, baseDomain)
}

// CloudFlare API methods

func (cf *CloudFlareClient) makeRequest(method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, cf.BaseURL+path, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+cf.APIToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (cf *CloudFlareClient) getAllRecords(name, recordType string) []CFRecord {
	path := fmt.Sprintf("/zones/%s/dns_records?name=%s&type=%s", cf.ZoneID, name, recordType)

	resp, err := cf.makeRequest("GET", path, nil)
	if err != nil {
		log.Printf("Error getting records for %s: %v", name, err)
		return []CFRecord{}
	}
	defer resp.Body.Close()

	var result CFListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("Error decoding response: %v", err)
		return []CFRecord{}
	}

	if result.Success {
		return result.Result
	}

	return []CFRecord{}
}

func (cf *CloudFlareClient) deleteRecord(recordID, name, recordType string) bool {
	path := fmt.Sprintf("/zones/%s/dns_records/%s", cf.ZoneID, recordID)

	resp, err := cf.makeRequest("DELETE", path, nil)
	if err != nil {
		log.Printf("Error deleting record for %s: %v", name, err)
		return false
	}
	defer resp.Body.Close()

	var result CFSingleResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("Error decoding response: %v", err)
		return false
	}

	if result.Success {
		log.Printf("Deleted %s record for %s (ID: %s)", recordType, name, recordID)
		return true
	}

	return false
}
