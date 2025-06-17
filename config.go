package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
)

type ServiceConfig struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type Config struct {
	Services []ServiceConfig `json:"services"`
}

var DefaultServices = []ServiceConfig{
	{
		Name: "OData.org Demo",
		URL:  "https://services.odata.org/V2/OData/OData.svc",
	},
	{
		Name: "Northwind V3",
		URL:  "https://services.odata.org/V3/Northwind/Northwind.svc",
	},
	{
		Name: "TripPin (V4)",
		URL:  "https://services.odata.org/V4/TripPinServiceRW",
	},
}

func LoadConfig() []ServiceConfig {
	// Parse command line flags
	var url = flag.String("url", "", "OData service URL")
	var user = flag.String("user", "", "Username for authentication")
	var pass = flag.String("pass", "", "Password for authentication")
	flag.Parse()

	// Check environment variables
	envURL := os.Getenv("ODATA_URL")
	envUser := os.Getenv("ODATA_USER")
	envPass := os.Getenv("ODATA_PASS")

	// Priority: CLI args > env vars > config file > defaults
	var services []ServiceConfig

	// Try to load from odatanavigator.json
	if configServices := loadFromConfigFile(); configServices != nil {
		services = configServices
	} else {
		services = DefaultServices
	}

	// Override with environment variables if provided
	if envURL != "" {
		services = []ServiceConfig{{
			Name:     "Environment Service",
			URL:      envURL,
			Username: envUser,
			Password: envPass,
		}}
	}

	// Override with CLI arguments if provided
	if *url != "" {
		services = []ServiceConfig{{
			Name:     "CLI Service",
			URL:      *url,
			Username: *user,
			Password: *pass,
		}}
	}

	return services
}

func loadFromConfigFile() []ServiceConfig {
	file, err := os.Open("odatanavigator.json")
	if err != nil {
		return nil // File doesn't exist or can't be opened
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		fmt.Printf("Warning: Could not read config file: %v\n", err)
		return nil
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		fmt.Printf("Warning: Could not parse config file: %v\n", err)
		return nil
	}

	return config.Services
}

func GetServiceNames(services []ServiceConfig) []string {
	names := make([]string, len(services))
	for i, svc := range services {
		names[i] = svc.Name
	}
	return names
}