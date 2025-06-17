package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	BaseURL = "https://services.odata.org/V2/OData/OData.svc"
)

type ODataService struct {
	baseURL string
	client  *http.Client
}

// OData V2 response structures
type ODataV2Response struct {
	D []map[string]interface{} `json:"d"`
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

func (o *ODataService) GetEntitySets() ([]string, error) {
	// For V2, we'll use hardcoded entity sets since metadata parsing is complex
	return []string{
		"Categories",
		"Products", 
		"Suppliers",
		"Persons",
		"Advertisements",
		"ProductDetails",
	}, nil
}

func (o *ODataService) GetEntities(entitySet string, top int) ([]map[string]interface{}, error) {
	url := fmt.Sprintf("%s/%s?$top=%d&$format=json", o.baseURL, entitySet, top)
	
	resp, err := o.client.Get(url)
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

	var odataResp ODataV2Response
	if err := json.Unmarshal(body, &odataResp); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w\nBody: %s", err, string(body))
	}

	return odataResp.D, nil
}

func (o *ODataService) GetEntity(entitySet, id string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/%s(%s)?$format=json", o.baseURL, entitySet, id)
	
	resp, err := o.client.Get(url)
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
	// Try to find a good display field
	if name, ok := entity["Name"].(string); ok && name != "" {
		return name
	}
	if title, ok := entity["Title"].(string); ok && title != "" {
		return title
	}
	if desc, ok := entity["Description"].(string); ok && desc != "" {
		return desc
	}
	if id := entity["ID"]; id != nil {
		return fmt.Sprintf("ID: %v", id)
	}
	if id := entity["CategoryID"]; id != nil {
		return fmt.Sprintf("CategoryID: %v", id)
	}
	if id := entity["ProductID"]; id != nil {
		return fmt.Sprintf("ProductID: %v", id)
	}
	
	// For debugging, show all fields
	var fields []string
	for k, v := range entity {
		if v != nil && !strings.HasPrefix(k, "__") {
			fields = append(fields, fmt.Sprintf("%s:%v", k, v))
		}
	}
	if len(fields) > 0 {
		return fields[0]
	}
	
	return fmt.Sprintf("Entity (%d fields)", len(entity))
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