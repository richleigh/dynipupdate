package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// RFC1918 private IP ranges
var rfc1918Ranges = []string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
}

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

type CFError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type CFCreateUpdateRequest struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

// Config holds application configuration
type Config struct {
	CFAPIToken      string
	CFZoneID        string
	InternalDomain  string
	ExternalDomain  string
	IPv6Domain      string
	CombinedDomain  string
	InstanceID      string
	Proxied         bool
	StaleThreshold  int // seconds (for cleanup mode)
	CleanupInterval int // seconds (for cleanup mode)
}

// IPAddresses holds detected IP addresses
type IPAddresses struct {
	InternalIPv4 []string
	ExternalIPv4 string
	ExternalIPv6 string
}

func main() {
	log.SetFlags(log.LstdFlags)

	// Parse command-line flags
	cleanupMode := flag.Bool("cleanup", false, "Run in cleanup mode (monitors and removes stale DNS records)")
	flag.Parse()

	config := loadConfig(*cleanupMode)

	cf := &CloudFlareClient{
		APIToken: config.CFAPIToken,
		ZoneID:   config.CFZoneID,
		BaseURL:  "https://api.cloudflare.com/client/v4",
	}

	if *cleanupMode {
		runCleanupService(cf, config)
		return
	}

	// Update mode
	log.Println("Starting Dynamic DNS Updater")
	ips := detectIPs()

	successCount := 0
	totalCount := 0

	// Update internal IPv4 records (support multiple addresses)
	if len(ips.InternalIPv4) > 0 {
		// Get all existing records for the internal domain
		existingRecords := cf.getAllRecords(config.InternalDomain, "A")

		// Create a map of existing record contents for quick lookup
		existingIPs := make(map[string]string) // content -> recordID
		for _, record := range existingRecords {
			existingIPs[record.Content] = record.ID
		}

		// Create a map of detected IPs
		detectedIPs := make(map[string]bool)
		for _, ip := range ips.InternalIPv4 {
			detectedIPs[ip] = true
		}

		// Create/update records for each detected IP
		for _, ip := range ips.InternalIPv4 {
			totalCount++
			if cf.ensureRecordExists(config.InternalDomain, "A", ip, config.Proxied) {
				successCount++
			}
		}

		// Create/update heartbeat for this domain
		heartbeatName := heartbeatRecordName(config.InternalDomain)
		heartbeatData := heartbeatContent()
		totalCount++
		if cf.upsertRecord(heartbeatName, "TXT", heartbeatData, false) {
			successCount++
			log.Printf("Updated heartbeat for %s", config.InternalDomain)
		}

		// Delete stale records (IPs that exist in DNS but not in detected list)
		for content, recordID := range existingIPs {
			if !detectedIPs[content] {
				totalCount++
				log.Printf("Deleting stale internal IPv4 record: %s", content)
				if cf.deleteRecord(recordID, config.InternalDomain, "A") {
					successCount++
				}
			}
		}
	} else {
		// No internal IPs found - delete all existing records and heartbeat
		existingRecords := cf.getAllRecords(config.InternalDomain, "A")
		for _, record := range existingRecords {
			totalCount++
			log.Printf("No internal IPv4 addresses found - deleting record: %s", record.Content)
			if cf.deleteRecord(record.ID, config.InternalDomain, "A") {
				successCount++
			}
		}

		// Delete the heartbeat
		heartbeatName := heartbeatRecordName(config.InternalDomain)
		totalCount++
		if cf.deleteRecordIfExists(heartbeatName, "TXT") {
			successCount++
			log.Printf("Deleted heartbeat for %s", config.InternalDomain)
		}
	}

	// Update external IPv4 record
	totalCount++
	if ips.ExternalIPv4 != "" {
		if cf.upsertRecord(config.ExternalDomain, "A", ips.ExternalIPv4, config.Proxied) {
			successCount++
			log.Printf("Updated external IPv4: %s -> %s", config.ExternalDomain, ips.ExternalIPv4)
		}
	} else {
		log.Println("No external IPv4 address found - deleting any existing record")
		if cf.deleteRecordIfExists(config.ExternalDomain, "A") {
			successCount++
		}
	}

	// Update external IPv6 record
	totalCount++
	if ips.ExternalIPv6 != "" {
		if cf.upsertRecord(config.IPv6Domain, "AAAA", ips.ExternalIPv6, config.Proxied) {
			successCount++
			log.Printf("Updated external IPv6: %s -> %s", config.IPv6Domain, ips.ExternalIPv6)
		}
	} else {
		log.Println("No external IPv6 address found - deleting any existing record")
		if cf.deleteRecordIfExists(config.IPv6Domain, "AAAA") {
			successCount++
		}
	}

	// Update combined domain (all IPs aggregated into one domain)
	if config.CombinedDomain != "" {
		log.Printf("Updating combined domain: %s", config.CombinedDomain)

		// Collect all IPv4 addresses (internal + external)
		var allIPv4s []string
		allIPv4s = append(allIPv4s, ips.InternalIPv4...)
		if ips.ExternalIPv4 != "" {
			allIPv4s = append(allIPv4s, ips.ExternalIPv4)
		}

		// Update A records for all IPv4s
		if len(allIPv4s) > 0 {
			// Get all existing A records for the combined domain
			existingRecords := cf.getAllRecords(config.CombinedDomain, "A")

			// Create a map of existing record contents for quick lookup
			existingIPs := make(map[string]string) // content -> recordID
			for _, record := range existingRecords {
				existingIPs[record.Content] = record.ID
			}

			// Create a map of detected IPs
			detectedIPs := make(map[string]bool)
			for _, ip := range allIPv4s {
				detectedIPs[ip] = true
			}

			// Create/update records for each IPv4
			for _, ip := range allIPv4s {
				totalCount++
				if cf.ensureRecordExists(config.CombinedDomain, "A", ip, config.Proxied) {
					successCount++
				}
			}

			// Delete stale A records (IPs that exist in DNS but not in detected list)
			for content, recordID := range existingIPs {
				if !detectedIPs[content] {
					totalCount++
					log.Printf("Deleting stale combined domain A record: %s", content)
					if cf.deleteRecord(recordID, config.CombinedDomain, "A") {
						successCount++
					}
				}
			}
		} else {
			// No IPv4s found - delete all A records
			existingRecords := cf.getAllRecords(config.CombinedDomain, "A")
			for _, record := range existingRecords {
				totalCount++
				log.Printf("No IPv4 addresses found - deleting combined domain A record: %s", record.Content)
				if cf.deleteRecord(record.ID, config.CombinedDomain, "A") {
					successCount++
				}
			}
		}

		// Update AAAA record for external IPv6
		totalCount++
		if ips.ExternalIPv6 != "" {
			if cf.upsertRecord(config.CombinedDomain, "AAAA", ips.ExternalIPv6, config.Proxied) {
				successCount++
				log.Printf("Updated combined domain IPv6: %s -> %s", config.CombinedDomain, ips.ExternalIPv6)
			}
		} else {
			log.Println("No external IPv6 address found - deleting combined domain AAAA record")
			if cf.deleteRecordIfExists(config.CombinedDomain, "AAAA") {
				successCount++
			}
		}
	}

	// Report results
	log.Printf("Completed: %d/%d records updated successfully\n", successCount, totalCount)

	if successCount == totalCount && totalCount > 0 {
		log.Println("All updates successful!")
		os.Exit(0)
	} else if successCount > 0 {
		log.Println("Some updates failed")
		os.Exit(1)
	} else {
		log.Println("All updates failed")
		os.Exit(1)
	}
}

func loadConfig(cleanupMode bool) *Config {
	apiToken := getEnvOrExit("CF_API_TOKEN")

	// Trim any whitespace that might have been included
	apiToken = strings.TrimSpace(apiToken)

	// Debug: Check for common issues
	if strings.HasPrefix(apiToken, "\"") || strings.HasPrefix(apiToken, "'") {
		log.Printf("WARNING: API token appears to have quotes around it (len=%d, first char=%q, last char=%q)",
			len(apiToken), apiToken[0], apiToken[len(apiToken)-1])
	}

	log.Printf("API token loaded (length: %d chars, starts with: %.8s..., ends with: ...%.4s)",
		len(apiToken), apiToken, apiToken[max(0, len(apiToken)-4):])

	config := &Config{
		CFAPIToken:      apiToken,
		CFZoneID:        getEnvOrExit("CF_ZONE_ID"),
		InternalDomain:  os.Getenv("INTERNAL_DOMAIN"),
		ExternalDomain:  os.Getenv("EXTERNAL_DOMAIN"),
		IPv6Domain:      os.Getenv("IPV6_DOMAIN"),
		CombinedDomain:  os.Getenv("COMBINED_DOMAIN"),
		Proxied:         strings.ToLower(os.Getenv("CF_PROXIED")) == "true",
		StaleThreshold:  getEnvOrDefaultInt("STALE_THRESHOLD_SECONDS", 3600), // 1 hour
		CleanupInterval: getEnvOrDefaultInt("CLEANUP_INTERVAL_SECONDS", 300), // 5 minutes
	}

	// In cleanup mode, domains are optional (cleanup whichever are configured)
	// In update mode, at least one domain must be configured
	if !cleanupMode {
		if config.InternalDomain == "" && config.ExternalDomain == "" &&
		   config.IPv6Domain == "" && config.CombinedDomain == "" {
			log.Fatal("At least one domain must be configured (INTERNAL_DOMAIN, EXTERNAL_DOMAIN, IPV6_DOMAIN, or COMBINED_DOMAIN)")
		}
	}

	config := &Config{
		CFAPIToken:      apiToken,
		CFZoneID:        getEnvOrExit("CF_ZONE_ID"),
		InternalDomain:  os.Getenv("INTERNAL_DOMAIN"),
		ExternalDomain:  os.Getenv("EXTERNAL_DOMAIN"),
		IPv6Domain:      os.Getenv("IPV6_DOMAIN"),
		CombinedDomain:  os.Getenv("COMBINED_DOMAIN"),
		InstanceID:      getEnvOrDefault("INSTANCE_ID", machineHostname),
		Proxied:         strings.ToLower(os.Getenv("CF_PROXIED")) == "true",
		StaleThreshold:  getEnvOrDefaultInt("STALE_THRESHOLD_SECONDS", 3600), // 1 hour
		CleanupInterval: getEnvOrDefaultInt("CLEANUP_INTERVAL_SECONDS", 300), // 5 minutes
	}

	// In cleanup mode, domains are optional (cleanup whichever are configured)
	// In update mode, at least one domain must be configured
	if !cleanupMode {
		if config.InternalDomain == "" && config.ExternalDomain == "" &&
		   config.IPv6Domain == "" && config.CombinedDomain == "" {
			log.Fatal("At least one domain must be configured (INTERNAL_DOMAIN, EXTERNAL_DOMAIN, IPV6_DOMAIN, or COMBINED_DOMAIN)")
		}
	}

	if cleanupMode {
		log.Printf("Cleanup Configuration:")
		log.Printf("  Internal Domain: %s", config.InternalDomain)
		log.Printf("  External Domain: %s", config.ExternalDomain)
		log.Printf("  IPv6 Domain: %s", config.IPv6Domain)
		log.Printf("  Combined Domain: %s", config.CombinedDomain)
		log.Printf("  Stale Threshold: %d seconds", config.StaleThreshold)
		log.Printf("  Cleanup Interval: %d seconds", config.CleanupInterval)
	}

	return config
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// heartbeatRecordName returns the domain name for the heartbeat TXT record
// The heartbeat is stored as a TXT record at the same name as the A/AAAA records
// Example: "anubis.i.4.bees.wtf" -> "anubis.i.4.bees.wtf" (same name, different type)
func heartbeatRecordName(domain string) string {
	return domain
}

// heartbeatContent creates the TXT record content with current timestamp
// Format: "timestamp" (quoted string)
func heartbeatContent() string {
	timestamp := time.Now().Unix()
	// TXT records should be quoted strings - just the timestamp
	return fmt.Sprintf("\"%d\"", timestamp)
}

func getEnvOrExit(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("Required environment variable %s not set", key)
	}
	return value
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
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

func detectIPs() *IPAddresses {
	ips := &IPAddresses{
		InternalIPv4: getInternalIPv4(),
		ExternalIPv4: getExternalIPv4(),
		ExternalIPv6: getExternalIPv6(),
	}
	return ips
}

func getInternalIPv4() []string {
	// Parse RFC1918 ranges
	var privateNets []*net.IPNet
	for _, cidr := range rfc1918Ranges {
		_, ipNet, _ := net.ParseCIDR(cidr)
		privateNets = append(privateNets, ipNet)
	}

	// Get all network interfaces
	interfaces, err := net.Interfaces()
	if err != nil {
		log.Printf("Error getting network interfaces: %v", err)
		return []string{}
	}

	var internalIPs []string
	seen := make(map[string]bool)

	// Check each interface for RFC1918 addresses
	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// Check if it's IPv4 and in RFC1918 range
			if ip == nil || ip.To4() == nil {
				continue
			}

			for _, privateNet := range privateNets {
				if privateNet.Contains(ip) {
					ipStr := ip.String()
					// Avoid duplicates
					if !seen[ipStr] {
						seen[ipStr] = true
						internalIPs = append(internalIPs, ipStr)
						log.Printf("Found internal IPv4: %s on interface %s", ipStr, iface.Name)
					}
				}
			}
		}
	}

	if len(internalIPs) == 0 {
		log.Println("No internal IPv4 addresses found")
	} else {
		log.Printf("Found %d internal IPv4 address(es)", len(internalIPs))
	}

	return internalIPs
}

func getExternalIPv4() string {
	// Use multiple services for redundancy
	services := []string{
		"https://api.ipify.org",
		"https://api4.ipify.org",
		"https://icanhazip.com",
		"https://ifconfig.me/ip",
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				// Force IPv4
				return (&net.Dialer{}).DialContext(ctx, "tcp4", addr)
			},
		},
	}

	for _, service := range services {
		resp, err := client.Get(service)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			continue
		}

		ipStr := strings.TrimSpace(string(body))
		// Validate it's an IPv4 address
		ip := net.ParseIP(ipStr)
		if ip != nil && ip.To4() != nil {
			log.Printf("Found external IPv4: %s", ipStr)
			return ipStr
		}
	}

	log.Println("Error detecting external IPv4")
	return ""
}

func getExternalIPv6() string {
	// Use multiple services for redundancy
	services := []string{
		"https://api6.ipify.org",
		"https://icanhazip.com",
		"https://ifconfig.me/ip",
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				// Force IPv6
				return (&net.Dialer{}).DialContext(ctx, "tcp6", addr)
			},
		},
	}

	for _, service := range services {
		resp, err := client.Get(service)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			continue
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			continue
		}

		ipStr := strings.TrimSpace(string(body))
		// Validate it's an IPv6 address
		ip := net.ParseIP(ipStr)
		if ip != nil && ip.To4() == nil && ip.To16() != nil {
			log.Printf("Found external IPv6: %s", ipStr)
			return ipStr
		}
	}

	log.Println("Error detecting external IPv6")
	return ""
}

// CloudFlareAPI defines the interface for CloudFlare DNS operations
type CloudFlareAPI interface {
	getRecordID(name, recordType string) string
	getRecord(name, recordType string) *CFRecord
	getAllRecords(name, recordType string) []CFRecord
	createRecord(name, recordType, content string, proxied bool) bool
	updateRecord(recordID, name, recordType, content string, proxied bool) bool
	deleteRecord(recordID, name, recordType string) bool
	deleteRecordIfExists(name, recordType string) bool
	upsertRecord(name, recordType, content string, proxied bool) bool
	ensureRecordExists(name, recordType, content string, proxied bool) bool
}

// CloudFlareClient handles CloudFlare API interactions
type CloudFlareClient struct {
	APIToken string
	ZoneID   string
	BaseURL  string
}

// Verify CloudFlareClient implements CloudFlareAPI
var _ CloudFlareAPI = (*CloudFlareClient)(nil)

// formatErrors converts CloudFlare error messages from json.RawMessage to readable strings
func formatErrors(errors []json.RawMessage) string {
	if len(errors) == 0 {
		return "unknown error"
	}

	var errorStrings []string
	for _, err := range errors {
		errorStrings = append(errorStrings, string(err))
	}
	return strings.Join(errorStrings, ", ")
}

func (cf *CloudFlareClient) makeRequest(method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, cf.BaseURL+path, body)
	if err != nil {
		return nil, err
	}

	authHeader := "Bearer " + cf.APIToken
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")

	// Debug: Log request details (without full token)
	log.Printf("API Request: %s %s (token length: %d, auth header length: %d)",
		method, path, len(cf.APIToken), len(authHeader))

	// Use a client with timeout instead of context
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	// Log response status for debugging
	if resp.StatusCode != http.StatusOK {
		log.Printf("API Response: %s (status: %d %s)", path, resp.StatusCode, resp.Status)
	}

	return resp, nil
}

func (cf *CloudFlareClient) getRecordID(name, recordType string) string {
	path := fmt.Sprintf("/zones/%s/dns_records?name=%s&type=%s", cf.ZoneID, name, recordType)

	resp, err := cf.makeRequest("GET", path, nil)
	if err != nil {
		log.Printf("Error getting record ID for %s: %v", name, err)
		return ""
	}
	defer resp.Body.Close()

	var result CFListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("Error decoding response: %v", err)
		return ""
	}

	if result.Success && len(result.Result) > 0 {
		return result.Result[0].ID
	}

	return ""
}

// getRecord returns the full record details, or nil if not found
func (cf *CloudFlareClient) getRecord(name, recordType string) *CFRecord {
	path := fmt.Sprintf("/zones/%s/dns_records?name=%s&type=%s", cf.ZoneID, name, recordType)

	resp, err := cf.makeRequest("GET", path, nil)
	if err != nil {
		log.Printf("Error getting record for %s: %v", name, err)
		return nil
	}
	defer resp.Body.Close()

	var result CFListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("Error decoding response: %v", err)
		return nil
	}

	if result.Success && len(result.Result) > 0 {
		return &result.Result[0]
	}

	return nil
}

// getAllRecords returns all records matching the name and type
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

// getAllRecordsByType returns all records in the zone matching the type (no name filter)
func (cf *CloudFlareClient) getAllRecordsByType(recordType string) []CFRecord {
	path := fmt.Sprintf("/zones/%s/dns_records?type=%s&per_page=1000", cf.ZoneID, recordType)

	resp, err := cf.makeRequest("GET", path, nil)
	if err != nil {
		log.Printf("Error getting all %s records: %v", recordType, err)
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

func (cf *CloudFlareClient) createRecord(name, recordType, content string, proxied bool) bool {
	path := fmt.Sprintf("/zones/%s/dns_records", cf.ZoneID)

	reqBody := CFCreateUpdateRequest{
		Type:    recordType,
		Name:    name,
		Content: content,
		TTL:     120, // 2 minutes for dynamic DNS
		Proxied: proxied,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		log.Printf("Error marshaling request: %v", err)
		return false
	}

	resp, err := cf.makeRequest("POST", path, strings.NewReader(string(jsonData)))
	if err != nil {
		log.Printf("Error creating record for %s: %v", name, err)
		return false
	}
	defer resp.Body.Close()

	var result CFSingleResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("Error decoding response: %v", err)
		return false
	}

	if result.Success {
		log.Printf("Created %s record for %s -> %s", recordType, name, content)
		return true
	}

	// Check if record already exists (error code 81058)
	for _, errRaw := range result.Errors {
		var cfErr CFError
		if err := json.Unmarshal(errRaw, &cfErr); err == nil {
			if cfErr.Code == 81058 {
				// Record already exists - try to get its ID and update instead
				log.Printf("Record already exists for %s, attempting update...", name)
				recordID := cf.getRecordID(name, recordType)
				if recordID != "" {
					return cf.updateRecord(recordID, name, recordType, content, proxied)
				}
				log.Printf("Failed to get record ID for existing record: %s", name)
				return false
			}
		}
	}

	log.Printf("Failed to create record: %s", formatErrors(result.Errors))
	return false
}

func (cf *CloudFlareClient) updateRecord(recordID, name, recordType, content string, proxied bool) bool {
	path := fmt.Sprintf("/zones/%s/dns_records/%s", cf.ZoneID, recordID)

	reqBody := CFCreateUpdateRequest{
		Type:    recordType,
		Name:    name,
		Content: content,
		TTL:     120,
		Proxied: proxied,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		log.Printf("Error marshaling request: %v", err)
		return false
	}

	resp, err := cf.makeRequest("PUT", path, strings.NewReader(string(jsonData)))
	if err != nil {
		log.Printf("Error updating record for %s: %v", name, err)
		return false
	}
	defer resp.Body.Close()

	var result CFSingleResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("Error decoding response: %v", err)
		return false
	}

	if result.Success {
		log.Printf("Updated %s record for %s -> %s", recordType, name, content)
		return true
	}

	log.Printf("Failed to update record: %s", formatErrors(result.Errors))
	return false
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
		log.Printf("Deleted %s record for %s", recordType, name)
		return true
	}

	log.Printf("Failed to delete record: %s", formatErrors(result.Errors))
	return false
}

func (cf *CloudFlareClient) deleteRecordIfExists(name, recordType string) bool {
	recordID := cf.getRecordID(name, recordType)
	if recordID != "" {
		return cf.deleteRecord(recordID, name, recordType)
	}
	return true
}

func (cf *CloudFlareClient) upsertRecord(name, recordType, content string, proxied bool) bool {
	record := cf.getRecord(name, recordType)
	if record != nil {
		// Record exists - check if content has changed
		if record.Content == content {
			log.Printf("No change needed for %s record %s (already %s)", recordType, name, content)
			return true
		}
		log.Printf("Content changed for %s record %s: %s -> %s", recordType, name, record.Content, content)
		return cf.updateRecord(record.ID, name, recordType, content, proxied)
	}
	return cf.createRecord(name, recordType, content, proxied)
}

// ensureRecordExists creates a record only if one with this exact content doesn't already exist.
// This is used for domains with multiple records of the same type (e.g., multiple A records).
func (cf *CloudFlareClient) ensureRecordExists(name, recordType, content string, proxied bool) bool {
	allRecords := cf.getAllRecords(name, recordType)

	// Check if a record with this specific content already exists
	for _, record := range allRecords {
		if record.Content == content {
			log.Printf("No change needed for %s record %s (already %s)", recordType, name, content)
			return true
		}
	}

	// Record with this content doesn't exist - create it
	return cf.createRecord(name, recordType, content, proxied)
}

// Cleanup service functions

func runCleanupService(cf *CloudFlareClient, config *Config) {
	log.Println("Starting DNS Cleanup Service")

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
		deleted := cleanupDomain(cf, config.InternalDomain, "A", config.StaleThreshold)
		totalDeleted += deleted
		log.Printf("Deleted %d stale A records from %s", deleted, config.InternalDomain)
	}

	// Cleanup external domain (A records only) if configured
	if config.ExternalDomain != "" {
		deleted := cleanupDomain(cf, config.ExternalDomain, "A", config.StaleThreshold)
		totalDeleted += deleted
		log.Printf("Deleted %d stale A records from %s", deleted, config.ExternalDomain)
	}

	// Cleanup IPv6 domain (AAAA records only) if configured
	if config.IPv6Domain != "" {
		deleted := cleanupDomain(cf, config.IPv6Domain, "AAAA", config.StaleThreshold)
		totalDeleted += deleted
		log.Printf("Deleted %d stale AAAA records from %s", deleted, config.IPv6Domain)
	}

	// Cleanup combined domain (both A and AAAA records) if configured
	if config.CombinedDomain != "" {
		deletedA := cleanupDomain(cf, config.CombinedDomain, "A", config.StaleThreshold)
		deletedAAAA := cleanupDomain(cf, config.CombinedDomain, "AAAA", config.StaleThreshold)
		totalDeleted += deletedA + deletedAAAA
		log.Printf("Deleted %d stale A and %d stale AAAA records from %s", deletedA, deletedAAAA, config.CombinedDomain)
	}

	log.Printf("Cleanup cycle complete. Total deleted: %d", totalDeleted)
}

func cleanupDomain(cf *CloudFlareClient, domain string, recordType string, staleThresholdSeconds int) int {
	deletedCount := 0

	if domain == "" {
		return 0
	}

	// Check the heartbeat for this domain
	heartbeatName := heartbeatRecordName(domain)
	heartbeatRecords := cf.getAllRecords(heartbeatName, "TXT")

	shouldDelete := false
	deleteReason := ""

	if len(heartbeatRecords) == 0 {
		// No heartbeat found - domain is stale
		shouldDelete = true
		deleteReason = "no heartbeat found"
	} else {
		// Parse the heartbeat content: "timestamp,instanceID"
		heartbeatContent := heartbeatRecords[0].Content
		// Remove quotes if present (CloudFlare returns TXT records with quotes)
		heartbeatContent = strings.Trim(heartbeatContent, "\"")

		parts := strings.Split(heartbeatContent, ",")
		if len(parts) < 1 {
			log.Printf("Invalid heartbeat format for %s: %s", domain, heartbeatContent)
			shouldDelete = true
			deleteReason = "invalid heartbeat format"
		} else {
			timestamp, err := strconv.ParseInt(parts[0], 10, 64)
			if err != nil {
				log.Printf("Invalid timestamp in heartbeat for %s: %s", domain, parts[0])
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
		log.Printf("Deleting %s %s records (%s)", domain, recordType, deleteReason)

		// Delete all records of this type for the domain
		records := cf.getAllRecords(domain, recordType)
		for _, record := range records {
			if cf.deleteRecord(record.ID, record.Name, recordType) {
				deletedCount++
				log.Printf("  Deleted %s record: %s -> %s", recordType, record.Name, record.Content)
			}
		}

		// Delete the heartbeat TXT record
		if len(heartbeatRecords) > 0 {
			cf.deleteRecord(heartbeatRecords[0].ID, heartbeatName, "TXT")
			log.Printf("  Deleted heartbeat: %s", heartbeatName)
		}
	}

	return deletedCount
}
