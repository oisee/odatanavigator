package main

type ServiceConfig struct {
	Name string
	URL  string
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

func GetServiceNames() []string {
	names := make([]string, len(DefaultServices))
	for i, svc := range DefaultServices {
		names[i] = svc.Name
	}
	return names
}