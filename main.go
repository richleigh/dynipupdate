package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
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
type CFResponse struct {
	Success bool              `json:"success"`
	Errors  []json.RawMessage `json:"errors"`
	Result  []CFRecord        `json:"result"`
}

type CFRecord struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
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
	CFAPIToken     string
	CFZoneID       string
	Hostname       string
	InternalDomain string
	ExternalDomain string
	IPv6Domain     string
	Proxied        bool
}

// IPAddresses holds detected IP addresses
type IPAddresses struct {
	InternalIPv4 string
	ExternalIPv4 string
	ExternalIPv6 string
}

func main() {
	log.SetFlags(log.LstdFlags)
	log.Println("Starting Dynamic DNS Updater")

	config := loadConfig()
	ips := detectIPs()

	cf := &CloudFlareClient{
		APIToken: config.CFAPIToken,
		ZoneID:   config.CFZoneID,
		BaseURL:  "https://api.cloudflare.com/client/v4",
	}

	successCount := 0
	totalCount := 0

	// Update internal IPv4 record
	totalCount++
	if ips.InternalIPv4 != "" {
		if cf.upsertRecord(config.InternalDomain, "A", ips.InternalIPv4, config.Proxied) {
			successCount++
		}
	} else {
		log.Println("No internal IPv4 address found - deleting any existing record")
		if cf.deleteRecordIfExists(config.InternalDomain, "A") {
			successCount++
		}
	}

	// Update external IPv4 record
	totalCount++
	if ips.ExternalIPv4 != "" {
		if cf.upsertRecord(config.ExternalDomain, "A", ips.ExternalIPv4, config.Proxied) {
			successCount++
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
		}
	} else {
		log.Println("No external IPv6 address found - deleting any existing record")
		if cf.deleteRecordIfExists(config.IPv6Domain, "AAAA") {
			successCount++
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

func loadConfig() *Config {
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
		CFAPIToken: apiToken,
		CFZoneID:   getEnvOrExit("CF_ZONE_ID"),
		Hostname:   getEnvOrExit("HOSTNAME"),
	}

	config.InternalDomain = getEnvOrDefault("INTERNAL_DOMAIN", config.Hostname)
	config.ExternalDomain = getEnvOrDefault("EXTERNAL_DOMAIN", config.Hostname)
	config.IPv6Domain = getEnvOrDefault("IPV6_DOMAIN", config.Hostname)
	config.Proxied = strings.ToLower(os.Getenv("CF_PROXIED")) == "true"

	return config
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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

func detectIPs() *IPAddresses {
	ips := &IPAddresses{
		InternalIPv4: getInternalIPv4(),
		ExternalIPv4: getExternalIPv4(),
		ExternalIPv6: getExternalIPv6(),
	}
	return ips
}

func getInternalIPv4() string {
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
		return ""
	}

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
					log.Printf("Found internal IPv4: %s", ip.String())
					return ip.String()
				}
			}
		}
	}

	log.Println("No internal IPv4 address found")
	return ""
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

// CloudFlareClient handles CloudFlare API interactions
type CloudFlareClient struct {
	APIToken string
	ZoneID   string
	BaseURL  string
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method, cf.BaseURL+path, body)
	if err != nil {
		return nil, err
	}

	authHeader := "Bearer " + cf.APIToken
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")

	// Debug: Log request details (without full token)
	log.Printf("API Request: %s %s (token length: %d, auth header length: %d)",
		method, path, len(cf.APIToken), len(authHeader))

	resp, err := http.DefaultClient.Do(req)
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

	var result CFResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("Error decoding response: %v", err)
		return ""
	}

	if result.Success && len(result.Result) > 0 {
		return result.Result[0].ID
	}

	return ""
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

	var result CFResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("Error decoding response: %v", err)
		return false
	}

	if result.Success {
		log.Printf("Created %s record for %s -> %s", recordType, name, content)
		return true
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

	var result CFResponse
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

	var result CFResponse
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
	recordID := cf.getRecordID(name, recordType)
	if recordID != "" {
		return cf.updateRecord(recordID, name, recordType, content, proxied)
	}
	return cf.createRecord(name, recordType, content, proxied)
}
