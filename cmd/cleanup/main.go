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
	CombinedDomain  string
	StaleThreshold  int // seconds
	CleanupInterval int // seconds
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

	// Cleanup internal domain if configured
	if config.InternalDomain != "" {
		deleted := cleanupDomain(cf, config.InternalDomain, config.StaleThreshold)
		totalDeleted += deleted
		log.Printf("Deleted %d stale records from %s", deleted, config.InternalDomain)
	}

	// Cleanup combined domain if configured
	if config.CombinedDomain != "" {
		deleted := cleanupDomain(cf, config.CombinedDomain, config.StaleThreshold)
		totalDeleted += deleted
		log.Printf("Deleted %d stale records from %s", deleted, config.CombinedDomain)
	}

	log.Printf("Cleanup cycle complete. Total deleted: %d", totalDeleted)
}

func cleanupDomain(cf *CloudFlareClient, domain string, staleThresholdSeconds int) int {
	deletedCount := 0

	// Get all A records for this domain
	aRecords := cf.getAllRecords(domain, "A")

	for _, aRecord := range aRecords {
		ip := aRecord.Content

		// Get the heartbeat TXT record for this IP
		heartbeatName := heartbeatRecordName(ip, domain)
		heartbeatRecords := cf.getAllRecords(heartbeatName, "TXT")

		if len(heartbeatRecords) == 0 {
			// No heartbeat record - this IP is stale
			log.Printf("No heartbeat found for IP %s - deleting", ip)
			if cf.deleteRecord(aRecord.ID, domain, "A") {
				deletedCount++
			}
			continue
		}

		// Parse the heartbeat content: "timestamp,instanceID"
		heartbeatContent := heartbeatRecords[0].Content
		// Remove quotes if present (CloudFlare returns TXT records with quotes)
		heartbeatContent = strings.Trim(heartbeatContent, "\"")

		parts := strings.Split(heartbeatContent, ",")
		if len(parts) < 1 {
			log.Printf("Invalid heartbeat format for IP %s: %s", ip, heartbeatContent)
			continue
		}

		timestamp, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			log.Printf("Invalid timestamp in heartbeat for IP %s: %s", ip, parts[0])
			continue
		}

		// Check if heartbeat is stale
		age := time.Now().Unix() - timestamp
		if age > int64(staleThresholdSeconds) {
			instanceID := "unknown"
			if len(parts) >= 2 {
				instanceID = parts[1]
			}
			log.Printf("Stale heartbeat for IP %s (instance: %s, age: %ds) - deleting", ip, instanceID, age)

			// Delete the A record
			if cf.deleteRecord(aRecord.ID, domain, "A") {
				deletedCount++
			}

			// Delete the heartbeat TXT record
			if len(heartbeatRecords) > 0 {
				cf.deleteRecord(heartbeatRecords[0].ID, heartbeatName, "TXT")
			}
		}
	}

	return deletedCount
}

func loadConfig() *Config {
	apiToken := getEnvOrExit("CF_API_TOKEN")
	apiToken = strings.TrimSpace(apiToken)

	config := &Config{
		CFAPIToken:      apiToken,
		CFZoneID:        getEnvOrExit("CF_ZONE_ID"),
		InternalDomain:  os.Getenv("INTERNAL_DOMAIN"),
		CombinedDomain:  os.Getenv("COMBINED_DOMAIN"),
		StaleThreshold:  getEnvOrDefaultInt("STALE_THRESHOLD_SECONDS", 900),  // 15 minutes
		CleanupInterval: getEnvOrDefaultInt("CLEANUP_INTERVAL_SECONDS", 300), // 5 minutes
	}

	log.Printf("Configuration:")
	log.Printf("  Internal Domain: %s", config.InternalDomain)
	log.Printf("  Combined Domain: %s", config.CombinedDomain)
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

// ipToDNSLabel converts an IP address to a DNS-safe label
func ipToDNSLabel(ip string) string {
	return strings.ReplaceAll(ip, ".", "-")
}

// heartbeatRecordName creates the TXT record name for a heartbeat
func heartbeatRecordName(ip, baseDomain string) string {
	return fmt.Sprintf("_heartbeat-%s.%s", ipToDNSLabel(ip), baseDomain)
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
