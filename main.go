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

// Environment variable prefix for all configuration
const envPrefix = "BEES_IP_UPDATE_"

// Track which environment variables have been consumed
var consumedEnvVars = make(map[string]bool)

// RFC1918 private IP ranges
var rfc1918Ranges = []string{
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
}

// CustomIPRange represents a user-defined IP range to detect and publish
type CustomIPRange struct {
	CIDR   string // CIDR notation, e.g., "100.0.0.0/8"
	Domain string // DNS domain for this range, e.g., "host.vpn.example.com"
	Type   string // "A" for IPv4, "AAAA" for IPv6
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
	CFAPIToken       string
	CFZoneID         string
	InternalDomain   string
	ExternalDomain   string
	IPv6Domain       string
	CustomIPv4Ranges []CustomIPRange // User-defined IPv4 ranges
	CustomIPv6Ranges []CustomIPRange // User-defined IPv6 ranges
	CombinedDomain   string
	TopLevelDomain   string // CNAME alias pointing to CombinedDomain
	Proxied          bool
	StaleThreshold   int // seconds (for cleanup mode)
	CleanupInterval  int // seconds (for cleanup mode)
}

// IPAddresses holds detected IP addresses
type IPAddresses struct {
	InternalIPv4   []string
	ExternalIPv4   string
	ExternalIPv6   string
	CustomRangeIPs map[string][]string // domain -> detected IPs for that custom range
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
	ips := detectIPs(config)

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

	// Update custom IPv4 range records
	for _, customRange := range config.CustomIPv4Ranges {
		customIPs, exists := ips.CustomRangeIPs[customRange.Domain]

		if exists && len(customIPs) > 0 {
			// Get all existing records for this custom domain
			existingRecords := cf.getAllRecords(customRange.Domain, "A")

			// Create a map of existing record contents for quick lookup
			existingIPs := make(map[string]string) // content -> recordID
			for _, record := range existingRecords {
				existingIPs[record.Content] = record.ID
			}

			// Create a map of detected IPs
			detectedIPs := make(map[string]bool)
			for _, ip := range customIPs {
				detectedIPs[ip] = true
			}

			// Create/update records for each detected IP
			for _, ip := range customIPs {
				totalCount++
				if cf.ensureRecordExists(customRange.Domain, "A", ip, config.Proxied) {
					successCount++
				}
			}

			// Create/update heartbeat for this domain
			heartbeatName := heartbeatRecordName(customRange.Domain)
			heartbeatData := heartbeatContent()
			totalCount++
			if cf.upsertRecord(heartbeatName, "TXT", heartbeatData, false) {
				successCount++
				log.Printf("Updated heartbeat for %s", customRange.Domain)
			}

			// Delete stale records (IPs that exist in DNS but not in detected list)
			for content, recordID := range existingIPs {
				if !detectedIPs[content] {
					totalCount++
					log.Printf("Deleting stale custom range IPv4 record: %s", content)
					if cf.deleteRecord(recordID, customRange.Domain, "A") {
						successCount++
					}
				}
			}
		} else {
			// No IPs found for this custom range - delete all existing records and heartbeat
			existingRecords := cf.getAllRecords(customRange.Domain, "A")
			for _, record := range existingRecords {
				totalCount++
				log.Printf("No IPs found in custom range %s - deleting record: %s", customRange.CIDR, record.Content)
				if cf.deleteRecord(record.ID, customRange.Domain, "A") {
					successCount++
				}
			}

			// Delete the heartbeat
			heartbeatName := heartbeatRecordName(customRange.Domain)
			totalCount++
			if cf.deleteRecordIfExists(heartbeatName, "TXT") {
				successCount++
				log.Printf("Deleted heartbeat for %s", customRange.Domain)
			}
		}
	}

	// Update custom IPv6 range records
	for _, customRange := range config.CustomIPv6Ranges {
		customIPs, exists := ips.CustomRangeIPs[customRange.Domain]

		if exists && len(customIPs) > 0 {
			// Get all existing records for this custom domain
			existingRecords := cf.getAllRecords(customRange.Domain, "AAAA")

			// Create a map of existing record contents for quick lookup
			existingIPs := make(map[string]string) // content -> recordID
			for _, record := range existingRecords {
				existingIPs[record.Content] = record.ID
			}

			// Create a map of detected IPs
			detectedIPs := make(map[string]bool)
			for _, ip := range customIPs {
				detectedIPs[ip] = true
			}

			// Create/update records for each detected IP
			for _, ip := range customIPs {
				totalCount++
				if cf.ensureRecordExists(customRange.Domain, "AAAA", ip, config.Proxied) {
					successCount++
				}
			}

			// Create/update heartbeat for this domain
			heartbeatName := heartbeatRecordName(customRange.Domain)
			heartbeatData := heartbeatContent()
			totalCount++
			if cf.upsertRecord(heartbeatName, "TXT", heartbeatData, false) {
				successCount++
				log.Printf("Updated heartbeat for %s", customRange.Domain)
			}

			// Delete stale records (IPs that exist in DNS but not in detected list)
			for content, recordID := range existingIPs {
				if !detectedIPs[content] {
					totalCount++
					log.Printf("Deleting stale custom range IPv6 record: %s", content)
					if cf.deleteRecord(recordID, customRange.Domain, "AAAA") {
						successCount++
					}
				}
			}
		} else {
			// No IPs found for this custom range - delete all existing records and heartbeat
			existingRecords := cf.getAllRecords(customRange.Domain, "AAAA")
			for _, record := range existingRecords {
				totalCount++
				log.Printf("No IPs found in custom range %s - deleting record: %s", customRange.CIDR, record.Content)
				if cf.deleteRecord(record.ID, customRange.Domain, "AAAA") {
					successCount++
				}
			}

			// Delete the heartbeat
			heartbeatName := heartbeatRecordName(customRange.Domain)
			totalCount++
			if cf.deleteRecordIfExists(heartbeatName, "TXT") {
				successCount++
				log.Printf("Deleted heartbeat for %s", customRange.Domain)
			}
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

		// Collect all IPv4 addresses (internal + custom ranges + external)
		var allIPv4s []string
		allIPv4s = append(allIPv4s, ips.InternalIPv4...)

		// Add all custom IPv4 range IPs
		for _, customRange := range config.CustomIPv4Ranges {
			if customIPs, exists := ips.CustomRangeIPs[customRange.Domain]; exists {
				allIPv4s = append(allIPv4s, customIPs...)
			}
		}

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

	// Update top-level CNAME alias (points to combined domain)
	if config.TopLevelDomain != "" && config.CombinedDomain != "" {
		log.Printf("Updating top-level CNAME alias: %s", config.TopLevelDomain)

		// Create/update CNAME record pointing to combined domain
		totalCount++
		if cf.upsertRecord(config.TopLevelDomain, "CNAME", config.CombinedDomain, config.Proxied) {
			successCount++
			log.Printf("Updated CNAME: %s -> %s", config.TopLevelDomain, config.CombinedDomain)
		}

		// Create/update heartbeat for top-level domain
		heartbeatName := heartbeatRecordName(config.TopLevelDomain)
		heartbeatData := heartbeatContent()
		totalCount++
		if cf.upsertRecord(heartbeatName, "TXT", heartbeatData, false) {
			successCount++
			log.Printf("Updated heartbeat for %s", config.TopLevelDomain)
		}
	} else if config.TopLevelDomain != "" && config.CombinedDomain == "" {
		log.Println("WARNING: TOP_LEVEL_DOMAIN is set but COMBINED_DOMAIN is not - skipping CNAME creation")
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

	// Parse custom IP ranges (supports up to 20 ranges for each type)
	customIPv4Ranges := parseCustomRanges("IPV4_RANGE", "A", 20)
	customIPv6Ranges := parseCustomRanges("IPV6_RANGE", "AAAA", 20)

	// Debug: Check for common issues
	if strings.HasPrefix(apiToken, "\"") || strings.HasPrefix(apiToken, "'") {
		log.Printf("WARNING: API token appears to have quotes around it (len=%d, first char=%q, last char=%q)",
			len(apiToken), apiToken[0], apiToken[len(apiToken)-1])
	}

	log.Printf("API token loaded (length: %d chars, starts with: %.8s..., ends with: ...%.4s)",
		len(apiToken), apiToken, apiToken[max(0, len(apiToken)-4):])

	config := &Config{
		CFAPIToken:       apiToken,
		CFZoneID:         getEnvOrExit("CF_ZONE_ID"),
		InternalDomain:   getEnv("INTERNAL_DOMAIN"),
		ExternalDomain:   getEnv("EXTERNAL_DOMAIN"),
		IPv6Domain:       getEnv("IPV6_DOMAIN"),
		CustomIPv4Ranges: customIPv4Ranges,
		CustomIPv6Ranges: customIPv6Ranges,
		CombinedDomain:   getEnv("COMBINED_DOMAIN"),
		TopLevelDomain:   getEnv("TOP_LEVEL_DOMAIN"),
		Proxied:          strings.ToLower(getEnv("CF_PROXIED")) == "true",
		StaleThreshold:   getEnvOrDefaultInt("STALE_THRESHOLD_SECONDS", 3600), // 1 hour
		CleanupInterval:  getEnvOrDefaultInt("CLEANUP_INTERVAL_SECONDS", 300), // 5 minutes
	}

	// At least one domain must be configured (both modes require this for safety)
	hasCustomRanges := len(config.CustomIPv4Ranges) > 0 || len(config.CustomIPv6Ranges) > 0
	if config.InternalDomain == "" && config.ExternalDomain == "" &&
		config.IPv6Domain == "" && !hasCustomRanges &&
		config.CombinedDomain == "" && config.TopLevelDomain == "" {
		log.Fatalf("At least one domain must be configured (%sINTERNAL_DOMAIN, %sEXTERNAL_DOMAIN, %sIPV6_DOMAIN, %sIPV4_RANGE_N/%sIPV6_RANGE_N, %sCOMBINED_DOMAIN, or %sTOP_LEVEL_DOMAIN)",
			envPrefix, envPrefix, envPrefix, envPrefix, envPrefix, envPrefix, envPrefix)
	}

	// Log configured custom ranges
	if len(customIPv4Ranges) > 0 {
		log.Printf("Configured %d custom IPv4 range(s):", len(customIPv4Ranges))
		for i, r := range customIPv4Ranges {
			log.Printf("  [%d] %s -> %s", i+1, r.CIDR, r.Domain)
		}
	}
	if len(customIPv6Ranges) > 0 {
		log.Printf("Configured %d custom IPv6 range(s):", len(customIPv6Ranges))
		for i, r := range customIPv6Ranges {
			log.Printf("  [%d] %s -> %s", i+1, r.CIDR, r.Domain)
		}
	}

	if cleanupMode {
		log.Printf("Cleanup Configuration:")
		log.Printf("  Stale Threshold: %d seconds", config.StaleThreshold)
		log.Printf("  Cleanup Interval: %d seconds", config.CleanupInterval)
		log.Printf("  Mode: Will only clean up configured managed domains")
	}

	// Validate that all BEES_IP_UPDATE_* env vars were consumed
	validateUnusedEnvVars()

	return config
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func getMapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
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

// getEnv gets an environment variable with the BEES_IP_UPDATE_ prefix and tracks consumption
func getEnv(key string) string {
	fullKey := envPrefix + key
	value := os.Getenv(fullKey)
	if value != "" {
		consumedEnvVars[fullKey] = true
	}
	return value
}

func getEnvOrExit(key string) string {
	value := getEnv(key)
	if value == "" {
		log.Fatalf("Required environment variable %s%s not set", envPrefix, key)
	}
	return value
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := getEnv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvOrDefaultInt(key string, defaultValue int) int {
	value := getEnv(key)
	if value == "" {
		return defaultValue
	}
	intValue, err := strconv.Atoi(value)
	if err != nil {
		log.Printf("Invalid integer value for %s%s: %s, using default %d", envPrefix, key, value, defaultValue)
		return defaultValue
	}
	return intValue
}

// validateUnusedEnvVars checks for any BEES_IP_UPDATE_* environment variables that were not consumed
// and logs warnings to help users debug configuration issues
func validateUnusedEnvVars() {
	allEnvVars := os.Environ()
	var unusedVars []string

	for _, envVar := range allEnvVars {
		// Split on first = to get key
		parts := strings.SplitN(envVar, "=", 2)
		if len(parts) < 2 {
			continue
		}
		key := parts[0]

		// Check if this is one of our prefixed vars
		if strings.HasPrefix(key, envPrefix) {
			// Check if it was consumed
			if !consumedEnvVars[key] {
				unusedVars = append(unusedVars, key)
			}
		}
	}

	if len(unusedVars) > 0 {
		log.Printf("WARNING: Found %d unused environment variable(s) with %s prefix:", len(unusedVars), envPrefix)
		for _, key := range unusedVars {
			// Remove prefix for cleaner display
			shortKey := strings.TrimPrefix(key, envPrefix)
			value := os.Getenv(key)

			// Provide helpful explanations for common issues
			explanation := getUnusedVarExplanation(shortKey, value)
			if explanation != "" {
				log.Printf("  - %s (value: %q) - %s", key, value, explanation)
			} else {
				log.Printf("  - %s (value: %q) - Unknown configuration variable", key, value)
			}
		}
		log.Printf("These variables had no effect. Check for typos or see documentation for valid variable names.")
	}
}

// getUnusedVarExplanation provides helpful explanations for why a variable wasn't used
func getUnusedVarExplanation(key, value string) string {
	// Check for common typos or misconfigurations
	switch {
	case strings.HasPrefix(key, "IPV4_RANGE_") && strings.HasSuffix(key, "_DOMAIN"):
		// Extract the number
		rangeNum := strings.TrimPrefix(key, "IPV4_RANGE_")
		rangeNum = strings.TrimSuffix(rangeNum, "_DOMAIN")
		cidrKey := fmt.Sprintf("IPV4_RANGE_%s", rangeNum)
		if getEnv(cidrKey) == "" {
			return fmt.Sprintf("Missing companion variable %s%s (both CIDR and DOMAIN must be set)", envPrefix, cidrKey)
		}
		return "Paired CIDR variable exists but may have validation issues"

	case strings.HasPrefix(key, "IPV4_RANGE_"):
		// Extract the number
		rangeNum := strings.TrimPrefix(key, "IPV4_RANGE_")
		domainKey := fmt.Sprintf("IPV4_RANGE_%s_DOMAIN", rangeNum)
		if getEnv(domainKey) == "" {
			return fmt.Sprintf("Missing companion variable %s%s (both CIDR and DOMAIN must be set)", envPrefix, domainKey)
		}
		// Check if CIDR is valid
		_, _, err := net.ParseCIDR(value)
		if err != nil {
			return fmt.Sprintf("Invalid CIDR notation: %v", err)
		}
		return "Paired DOMAIN variable exists but may have validation issues"

	case strings.HasPrefix(key, "IPV6_RANGE_") && strings.HasSuffix(key, "_DOMAIN"):
		rangeNum := strings.TrimPrefix(key, "IPV6_RANGE_")
		rangeNum = strings.TrimSuffix(rangeNum, "_DOMAIN")
		cidrKey := fmt.Sprintf("IPV6_RANGE_%s", rangeNum)
		if getEnv(cidrKey) == "" {
			return fmt.Sprintf("Missing companion variable %s%s (both CIDR and DOMAIN must be set)", envPrefix, cidrKey)
		}
		return "Paired CIDR variable exists but may have validation issues"

	case strings.HasPrefix(key, "IPV6_RANGE_"):
		rangeNum := strings.TrimPrefix(key, "IPV6_RANGE_")
		domainKey := fmt.Sprintf("IPV6_RANGE_%s_DOMAIN", rangeNum)
		if getEnv(domainKey) == "" {
			return fmt.Sprintf("Missing companion variable %s%s (both CIDR and DOMAIN must be set)", envPrefix, domainKey)
		}
		// Check if CIDR is valid
		_, _, err := net.ParseCIDR(value)
		if err != nil {
			return fmt.Sprintf("Invalid CIDR notation: %v", err)
		}
		return "Paired DOMAIN variable exists but may have validation issues"
	}

	return ""
}

// parseCustomRanges parses custom IP range configuration from environment variables
// Format: ${prefix}_1=${CIDR}, ${prefix}_1_DOMAIN=${domain}
// Example: IPV4_RANGE_1=100.0.0.0/8, IPV4_RANGE_1_DOMAIN=host.vpn.example.com
func parseCustomRanges(prefix string, recordType string, maxRanges int) []CustomIPRange {
	var ranges []CustomIPRange

	for i := 1; i <= maxRanges; i++ {
		cidrKey := fmt.Sprintf("%s_%d", prefix, i)
		domainKey := fmt.Sprintf("%s_%d_DOMAIN", prefix, i)

		cidr := getEnv(cidrKey)
		domain := getEnv(domainKey)

		// Both must be set for a valid range
		if cidr == "" && domain == "" {
			continue // Skip this index
		}

		if cidr == "" {
			log.Printf("WARNING: %s%s is set but %s%s is not - skipping", envPrefix, domainKey, envPrefix, cidrKey)
			continue
		}

		if domain == "" {
			log.Printf("WARNING: %s%s is set but %s%s is not - skipping", envPrefix, cidrKey, envPrefix, domainKey)
			continue
		}

		// Validate CIDR notation
		_, _, err := net.ParseCIDR(cidr)
		if err != nil {
			log.Printf("WARNING: Invalid CIDR notation in %s%s: %s (%v) - skipping", envPrefix, cidrKey, cidr, err)
			continue
		}

		ranges = append(ranges, CustomIPRange{
			CIDR:   cidr,
			Domain: domain,
			Type:   recordType,
		})
	}

	return ranges
}

func detectIPs(config *Config) *IPAddresses {
	ips := &IPAddresses{
		InternalIPv4:   getInternalIPv4(),
		ExternalIPv4:   getExternalIPv4(),
		ExternalIPv6:   getExternalIPv6(),
		CustomRangeIPs: make(map[string][]string),
	}

	// Detect IPs for custom IPv4 ranges
	for _, customRange := range config.CustomIPv4Ranges {
		detectedIPs := getIPsInRange(customRange.CIDR, customRange.Domain)
		if len(detectedIPs) > 0 {
			ips.CustomRangeIPs[customRange.Domain] = detectedIPs
		}
	}

	// Detect IPs for custom IPv6 ranges
	for _, customRange := range config.CustomIPv6Ranges {
		detectedIPs := getIPsInRange(customRange.CIDR, customRange.Domain)
		if len(detectedIPs) > 0 {
			ips.CustomRangeIPs[customRange.Domain] = detectedIPs
		}
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

// getIPsInRange detects IPs on network interfaces that fall within the specified CIDR range
// Supports both IPv4 and IPv6 ranges
func getIPsInRange(cidr string, domain string) []string {
	// Parse the CIDR
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		log.Printf("Error parsing CIDR %s: %v", cidr, err)
		return []string{}
	}

	// Get all network interfaces
	interfaces, err := net.Interfaces()
	if err != nil {
		log.Printf("Error getting network interfaces: %v", err)
		return []string{}
	}

	var foundIPs []string
	seen := make(map[string]bool)

	// Check each interface for matching addresses
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

			if ip == nil {
				continue
			}

			// Check if IP is in the specified range
			if ipNet.Contains(ip) {
				ipStr := ip.String()
				// Avoid duplicates
				if !seen[ipStr] {
					seen[ipStr] = true
					foundIPs = append(foundIPs, ipStr)
					log.Printf("Found IP in range %s: %s on interface %s (for domain %s)", cidr, ipStr, iface.Name, domain)
				}
			}
		}
	}

	if len(foundIPs) == 0 {
		log.Printf("No IPs found in range %s (for domain %s)", cidr, domain)
	} else {
		log.Printf("Found %d IP(s) in range %s (for domain %s)", len(foundIPs), cidr, domain)
	}

	return foundIPs
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

// DNSRecord represents a generic DNS record (provider-agnostic)
type DNSRecord struct {
	ID      string // Provider-specific record ID
	Type    string // A, AAAA, CNAME, TXT, etc.
	Name    string // Full domain name
	Content string // IP address or record content
}

// DNSProvider defines a generic interface for DNS operations
// This allows supporting multiple providers: CloudFlare, Route53, DigitalOcean, etc.
type DNSProvider interface {
	GetRecordID(name, recordType string) string
	GetRecord(name, recordType string) *DNSRecord
	GetAllRecords(name, recordType string) []DNSRecord
	CreateRecord(name, recordType, content string, proxied bool) bool
	UpdateRecord(recordID, name, recordType, content string, proxied bool) bool
	DeleteRecord(recordID, name, recordType string) bool
	DeleteRecordIfExists(name, recordType string) bool
	UpsertRecord(name, recordType, content string, proxied bool) bool
	EnsureRecordExists(name, recordType, content string, proxied bool) bool
}

// CloudFlareAPI defines the interface for CloudFlare DNS operations (deprecated, use DNSProvider)
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

// CloudFlareClient implements both DNSProvider and CloudFlareAPI
type CloudFlareClient struct {
	APIToken string
	ZoneID   string
	BaseURL  string
}

// Verify CloudFlareClient implements both interfaces
var _ CloudFlareAPI = (*CloudFlareClient)(nil)
var _ DNSProvider = (*CloudFlareClient)(nil)

// Helper functions to convert between CloudFlare-specific and generic types
func cfRecordToDNSRecord(cfr *CFRecord) *DNSRecord {
	if cfr == nil {
		return nil
	}
	return &DNSRecord{
		ID:      cfr.ID,
		Type:    cfr.Type,
		Name:    cfr.Name,
		Content: cfr.Content,
	}
}

func cfRecordsToDNSRecords(cfrs []CFRecord) []DNSRecord {
	records := make([]DNSRecord, len(cfrs))
	for i, cfr := range cfrs {
		records[i] = DNSRecord{
			ID:      cfr.ID,
			Type:    cfr.Type,
			Name:    cfr.Name,
			Content: cfr.Content,
		}
	}
	return records
}

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


// DNSProvider interface implementation (capitalized wrapper methods)

func (cf *CloudFlareClient) GetRecordID(name, recordType string) string {
	return cf.getRecordID(name, recordType)
}

func (cf *CloudFlareClient) GetRecord(name, recordType string) *DNSRecord {
	return cfRecordToDNSRecord(cf.getRecord(name, recordType))
}

func (cf *CloudFlareClient) GetAllRecords(name, recordType string) []DNSRecord {
	return cfRecordsToDNSRecords(cf.getAllRecords(name, recordType))
}

func (cf *CloudFlareClient) CreateRecord(name, recordType, content string, proxied bool) bool {
	return cf.createRecord(name, recordType, content, proxied)
}

func (cf *CloudFlareClient) UpdateRecord(recordID, name, recordType, content string, proxied bool) bool {
	return cf.updateRecord(recordID, name, recordType, content, proxied)
}

func (cf *CloudFlareClient) DeleteRecord(recordID, name, recordType string) bool {
	return cf.deleteRecord(recordID, name, recordType)
}

func (cf *CloudFlareClient) DeleteRecordIfExists(name, recordType string) bool {
	return cf.deleteRecordIfExists(name, recordType)
}

func (cf *CloudFlareClient) UpsertRecord(name, recordType, content string, proxied bool) bool {
	return cf.upsertRecord(name, recordType, content, proxied)
}

func (cf *CloudFlareClient) EnsureRecordExists(name, recordType, content string, proxied bool) bool {
	return cf.ensureRecordExists(name, recordType, content, proxied)
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

	// Build list of managed domains (only clean up domains we're responsible for)
	managedDomains := make(map[string]bool)
	if config.InternalDomain != "" {
		managedDomains[config.InternalDomain] = true
	}
	if config.ExternalDomain != "" {
		managedDomains[config.ExternalDomain] = true
	}
	if config.IPv6Domain != "" {
		managedDomains[config.IPv6Domain] = true
	}
	if config.CombinedDomain != "" {
		managedDomains[config.CombinedDomain] = true
	}
	if config.TopLevelDomain != "" {
		managedDomains[config.TopLevelDomain] = true
	}

	if len(managedDomains) == 0 {
		log.Fatal("ERROR: Cannot run cleanup mode without any configured domains. Set at least one of: INTERNAL_DOMAIN, EXTERNAL_DOMAIN, IPV6_DOMAIN, COMBINED_DOMAIN, or TOP_LEVEL_DOMAIN")
	}

	log.Printf("Cleanup will only affect these managed domains: %v", getMapKeys(managedDomains))

	// Get all TXT records in the zone (potential heartbeats)
	txtRecords := cf.getAllRecordsByType("TXT")
	log.Printf("Found %d TXT records in zone", len(txtRecords))

	totalDeleted := 0
	staleDomains := make(map[string]string) // domain -> reason

	// Check each TXT record to see if it's a heartbeat and if it's stale
	for _, txtRecord := range txtRecords {
		// SAFETY CHECK: Only consider domains we manage
		if !managedDomains[txtRecord.Name] {
			continue
		}

		content := strings.Trim(txtRecord.Content, "\"")

		// Try to parse as a timestamp (heartbeat format)
		timestamp, err := strconv.ParseInt(content, 10, 64)
		if err != nil {
			// Not a heartbeat (not a valid timestamp)
			continue
		}

		// Check if heartbeat is stale
		age := time.Now().Unix() - timestamp
		if age > int64(config.StaleThreshold) {
			staleDomains[txtRecord.Name] = fmt.Sprintf("stale heartbeat (age: %ds)", age)
		}
	}

	if len(staleDomains) == 0 {
		log.Println("No stale domains found")
		log.Printf("Cleanup cycle complete. Total deleted: 0")
		return
	}

	log.Printf("Found %d stale domain(s) to clean up", len(staleDomains))

	// Delete all records for stale domains
	for domain, reason := range staleDomains {
		log.Printf("Cleaning up stale domain: %s (%s)", domain, reason)

		// Delete A records
		aRecords := cf.getAllRecords(domain, "A")
		for _, record := range aRecords {
			if cf.deleteRecord(record.ID, record.Name, "A") {
				totalDeleted++
				log.Printf("  Deleted A record: %s -> %s", record.Name, record.Content)
			}
		}

		// Delete AAAA records
		aaaaRecords := cf.getAllRecords(domain, "AAAA")
		for _, record := range aaaaRecords {
			if cf.deleteRecord(record.ID, record.Name, "AAAA") {
				totalDeleted++
				log.Printf("  Deleted AAAA record: %s -> %s", record.Name, record.Content)
			}
		}

		// Delete CNAME records
		cnameRecords := cf.getAllRecords(domain, "CNAME")
		for _, record := range cnameRecords {
			if cf.deleteRecord(record.ID, record.Name, "CNAME") {
				totalDeleted++
				log.Printf("  Deleted CNAME record: %s -> %s", record.Name, record.Content)
			}
		}

		// Delete TXT heartbeat record
		txtRecords := cf.getAllRecords(domain, "TXT")
		for _, record := range txtRecords {
			if cf.deleteRecord(record.ID, record.Name, "TXT") {
				totalDeleted++
				log.Printf("  Deleted TXT heartbeat: %s", record.Name)
			}
		}
	}

	log.Printf("Cleanup cycle complete. Total deleted: %d records from %d domain(s)", totalDeleted, len(staleDomains))
}
