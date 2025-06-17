package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

const (
	BaseURL = "https://services.odata.org/V2/OData/OData.svc"
)

type ODataService struct {
	baseURL  string
	client   *http.Client
	username string
	password string
}

// OData V2 response structures
type ODataV2Response struct {
	D []map[string]interface{} `json:"d"`
}

// SAP OData V2 response structure (with results wrapper)
type SAPODataV2Response struct {
	D struct {
		Results []map[string]interface{} `json:"results"`
	} `json:"d"`
}

func NewODataService() *ODataService {
	return &ODataService{
		baseURL: BaseURL,
		client:  &http.Client{},
	}
}

func NewODataServiceWithURL(url string) *ODataService {
	return &ODataService{
		baseURL: url,
		client:  &http.Client{},
	}
}

func NewODataServiceWithAuth(url, username, password string) *ODataService {
	return &ODataService{
		baseURL:  url,
		client:   &http.Client{},
		username: username,
		password: password,
	}
}

func (o *ODataService) GetEntitySets() ([]string, error) {
	// First try to get metadata and parse entity sets
	metadataURL := strings.TrimSuffix(o.baseURL, "/") + "/$metadata"
	
	req, err := http.NewRequest("GET", metadataURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create metadata request: %w", err)
	}
	
	if o.username != "" && o.password != "" {
		req.SetBasicAuth(o.username, o.password)
	}
	
	resp, err := o.client.Do(req)
	if err != nil {
		// Fallback to hardcoded entity sets for demo services
		return []string{"Categories", "Products", "Suppliers", "Persons", "Advertisements", "ProductDetails"}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Fallback to hardcoded entity sets
		return []string{"Categories", "Products", "Suppliers", "Persons", "Advertisements", "ProductDetails"}, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata: %w", err)
	}

	// Parse entity sets from metadata using regex (simple approach)
	entitySets := parseEntitySetsFromMetadata(string(body))
	if len(entitySets) == 0 {
		// Fallback to hardcoded entity sets
		return []string{"Categories", "Products", "Suppliers", "Persons", "Advertisements", "ProductDetails"}, nil
	}


	return entitySets, nil
}

func parseEntitySetsFromMetadata(metadata string) []string {
	// Use regex to find EntitySet elements
	re := regexp.MustCompile(`<EntitySet[^>]+Name="([^"]+)"`)
	matches := re.FindAllStringSubmatch(metadata, -1)
	
	var entitySets []string
	for _, match := range matches {
		if len(match) > 1 {
			entitySets = append(entitySets, match[1])
		}
	}
	
	// Add function imports with [FUNC] prefix
	funcRe := regexp.MustCompile(`<FunctionImport[^>]+Name="([^"]+)"`)
	funcMatches := funcRe.FindAllStringSubmatch(metadata, -1)
	for _, match := range funcMatches {
		if len(match) > 1 {
			entitySets = append(entitySets, "[FUNC] "+match[1])
		}
	}
	
	return entitySets
}

func (o *ODataService) GetEntities(entitySet string, top int) ([]map[string]interface{}, error) {
	// Default to 10 if not specified
	if top <= 0 {
		top = 10
	}
	url := fmt.Sprintf("%s/%s?$top=%d&$format=json", o.baseURL, entitySet, top)
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	if o.username != "" && o.password != "" {
		req.SetBasicAuth(o.username, o.password)
	}
	
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch entities: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Try parsing as standard OData V2 first
	var odataResp ODataV2Response
	if err := json.Unmarshal(body, &odataResp); err == nil && len(odataResp.D) > 0 {
		return odataResp.D, nil
	}

	// Try parsing as SAP OData V2 (with results wrapper)
	var sapResp SAPODataV2Response
	if err := json.Unmarshal(body, &sapResp); err == nil {
		return sapResp.D.Results, nil
	}

	return nil, fmt.Errorf("failed to parse JSON: %w\nBody: %s", err, string(body))
}

// GetEntitiesWithCount returns entities and checks if there are more
func (o *ODataService) GetEntitiesWithCount(entitySet string, top int) (entities []map[string]interface{}, hasMore bool, err error) {
	// Default to 10 if not specified
	if top <= 0 {
		top = 10
	}
	// Request one extra to check if there are more
	entities, err = o.GetEntities(entitySet, top+1)
	if err != nil {
		return nil, false, err
	}
	
	// Check if we got more than requested
	if len(entities) > top {
		hasMore = true
		entities = entities[:top] // Return only requested amount
	}
	
	return entities, hasMore, nil
}

func (o *ODataService) GetEntity(entitySet, id string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/%s(%s)?$format=json", o.baseURL, entitySet, id)
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	if o.username != "" && o.password != "" {
		req.SetBasicAuth(o.username, o.password)
	}
	
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch entity: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		D map[string]interface{} `json:"d"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return result.D, nil
}

func formatEntityForDisplay(entity map[string]interface{}) string {
	// Extract entity type from metadata if available (for future use)
	_ = entity // avoid unused variable warning
	
	// Try to find key fields based on common patterns and entity type
	var keyValue string
	var additionalInfo string
	
	// Common key field patterns
	keyFields := []string{"Program", "Class", "Interface", "Package", "Function", 
		"ID", "Id", "Key", "Code", "Number", 
		"ProductID", "CategoryID", "CustomerID", "OrderID", "EmployeeID"}
	
	// Check for key fields
	for _, field := range keyFields {
		if val := entity[field]; val != nil {
			keyValue = fmt.Sprintf("%v", val)
			// Look for descriptive fields to append
			descFields := []string{"Title", "Name", "Description", "Text"}
			for _, descField := range descFields {
				if desc := entity[descField]; desc != nil && desc != "" {
					additionalInfo = fmt.Sprintf(" | %v", desc)
					break
				}
			}
			break
		}
	}
	
	// If no key found, use first non-metadata field
	if keyValue == "" {
		for k, v := range entity {
			if v != nil && !strings.HasPrefix(k, "__") {
				keyValue = fmt.Sprintf("%s: %v", k, v)
				break
			}
		}
	}
	
	if keyValue == "" {
		return fmt.Sprintf("Entity (%d fields)", len(entity))
	}
	
	return keyValue + additionalInfo
}

func formatEntityDetails(entity map[string]interface{}) []string {
	var details []string
	
	for key, value := range entity {
		if value != nil && !strings.HasPrefix(key, "__") {
			details = append(details, fmt.Sprintf("%s: %v", key, value))
		}
	}
	
	return details
}

type EntityCapabilities struct {
	Searchable  bool
	Filterable  bool
	Creatable   bool
	Updatable   bool
	Deletable   bool
	MediaType   bool
}

func GetEntitySetCapabilities(entitySet string) EntityCapabilities {
	// For demo purposes, return capabilities based on entity set
	// In a real implementation, this would parse the OData $metadata
	switch entitySet {
	case "Categories":
		return EntityCapabilities{
			Searchable: true,
			Filterable: true,
			Creatable:  true,
			Updatable:  true,
			Deletable:  true,
			MediaType:  false,
		}
	case "Products":
		return EntityCapabilities{
			Searchable: true,
			Filterable: true,
			Creatable:  true,
			Updatable:  true,
			Deletable:  false, // Products might not be deletable
			MediaType:  false,
		}
	case "Advertisements":
		return EntityCapabilities{
			Searchable: true,
			Filterable: true,
			Creatable:  true,
			Updatable:  true,
			Deletable:  true,
			MediaType:  true, // Advertisements might have media
		}
	default:
		return EntityCapabilities{
			Searchable: true,
			Filterable: true,
			Creatable:  false,
			Updatable:  false,
			Deletable:  false,
			MediaType:  false,
		}
	}
}

func (c EntityCapabilities) String() string {
	var caps []string
	if c.Searchable {
		caps = append(caps, "S")
	}
	if c.Filterable {
		caps = append(caps, "F")
	}
	if c.Creatable {
		caps = append(caps, "C")
	}
	if c.Updatable {
		caps = append(caps, "U")
	}
	if c.Deletable {
		caps = append(caps, "D")
	}
	if c.MediaType {
		caps = append(caps, "M")
	}
	return fmt.Sprintf("[%s]", strings.Join(caps, ""))
}