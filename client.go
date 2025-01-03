package bunny

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/libdns/libdns"
)

type getAllRecordsResponse struct {
	Records []bunnyRecord `json:"Records"`
}

type getAllZonesResponse struct {
	Zones []bunnyZone `json:"Items"`
}

type bunnyZone struct {
	ID int `json:"Id"`
}

type bunnyRecord struct {
	ID    int    `json:"Id,omitempty"`
	Type  int    `json:"Type"`
	Name  string `json:"Name"`
	Value string `json:"Value"`
	TTL   int    `json:"Ttl"`
}

func doRequest(accessKey string, request *http.Request) ([]byte, error) {
	request.Header.Add("accept", "application/json")
	request.Header.Add("AccessKey", accessKey)

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%s (%d)", http.StatusText(response.StatusCode), response.StatusCode)
	}

	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func getZoneID(ctx context.Context, zone string, accessKey string, domain string) (int, error) {
	param := domain
	if len(zone) != 0 {
		param = zone
	}

	// [page => 1] and [perPage => 5] are the smallest accepted values for the API
	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("https://api.bunny.net/dnszone?page=1&perPage=5&search=%s", url.QueryEscape(param)), nil)
	if err != nil {
		return 0, err
	}

	data, err := doRequest(accessKey, req)
	if err != nil {
		return 0, err
	}

	result := getAllZonesResponse{}
	if err := json.Unmarshal(data, &result); err != nil {
		return 0, err
	}

	if len(result.Zones) > 1 {
		return 0, errors.New("zone is ambiguous")
	}

	if len(result.Zones) != 1 {
		return 0, errors.New("unable to find zone")
	}

	return result.Zones[0].ID, nil
}

func getAllRecords(ctx context.Context, zone string, accessKey string, domain string) ([]libdns.Record, error) {
	zoneID, err := getZoneID(ctx, zone, accessKey, domain)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET",
		fmt.Sprintf("https://api.bunny.net/dnszone/%d", zoneID), nil)
	if err != nil {
		return nil, err
	}

	data, err := doRequest(accessKey, req)
	if err != nil {
		return nil, err
	}

	result := getAllRecordsResponse{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	records := []libdns.Record{}
	for _, resData := range result.Records {
		records = append(records, libdns.Record{
			ID:    fmt.Sprint(resData.ID),
			Type:  fromBunnyType(resData.Type),
			Name:  resData.Name,
			Value: resData.Value,
			TTL:   time.Duration(resData.TTL) * time.Second,
		})
	}

	return records, nil
}

func createRecord(ctx context.Context, zone string, accessKey string, domain string, record libdns.Record) (libdns.Record, error) {
	zoneID, err := getZoneID(ctx, zone, accessKey, domain)
	if err != nil {
		return libdns.Record{}, err
	}

	reqData := bunnyRecord{
		Type:  toBunnyType(record.Type),
		Name:  stripName(zone, domain, record.Name),
		Value: record.Value,
		TTL:   int(record.TTL.Seconds()),
	}

	reqBuffer, err := json.Marshal(reqData)
	if err != nil {
		return libdns.Record{}, err
	}

	req, err := http.NewRequestWithContext(ctx, "PUT",
		fmt.Sprintf("https://api.bunny.net/dnszone/%d/records", zoneID), bytes.NewBuffer(reqBuffer))
	if err != nil {
		return libdns.Record{}, err
	}

	req.Header.Add("content-type", "application/json")
	data, err := doRequest(accessKey, req)
	if err != nil {
		return libdns.Record{}, err
	}

	result := bunnyRecord{}
	if err := json.Unmarshal(data, &result); err != nil {
		return libdns.Record{}, err
	}

	return libdns.Record{
		ID:    fmt.Sprint(result.ID),
		Type:  fromBunnyType(result.Type),
		Name:  libdns.RelativeName(result.Name, zone),
		Value: result.Value,
		TTL:   time.Duration(result.TTL) * time.Second,
	}, nil
}

func deleteRecord(ctx context.Context, zone string, accessKey string, domain string, record libdns.Record) error {
	zoneID, err := getZoneID(ctx, zone, accessKey, domain)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "DELETE",
		fmt.Sprintf("https://api.bunny.net/dnszone/%d/records/%s", zoneID, url.PathEscape(record.ID)), nil)
	if err != nil {
		return err
	}

	_, err = doRequest(accessKey, req)
	if err != nil {
		return err
	}

	return nil
}

func stripName(zone string, domain string, name string) string {
	if len(name) != 0 {
		return name
	}

	if len(zone) != 0 {
		return strings.TrimSuffix(strings.TrimSuffix(strings.Clone(domain), zone), ".")
	}

	return domain
}

func updateRecord(ctx context.Context, zone string, accessKey string, domain string, record libdns.Record) error {
	zoneID, err := getZoneID(ctx, zone, accessKey, domain)
	if err != nil {
		return err
	}

	reqData := bunnyRecord{
		Type:  toBunnyType(record.Type),
		Name:  stripName(zone, domain, record.Name),
		Value: record.Value,
		TTL:   int(record.TTL.Seconds()),
	}

	reqBuffer, err := json.Marshal(reqData)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("https://api.bunny.net/dnszone/%d/records/%s", zoneID, url.PathEscape(record.ID)), bytes.NewBuffer(reqBuffer))
	if err != nil {
		return err
	}

	req.Header.Add("content-type", "application/json")

	_, err = doRequest(accessKey, req)
	if err != nil {
		return err
	}

	return nil
}

// Creates a new record if it does not exist, or updates an existing one.
func createOrUpdateRecord(ctx context.Context, zone string, accessKey string, domain string, record libdns.Record) (libdns.Record, error) {
	if len(record.ID) == 0 {
		return createRecord(ctx, zone, accessKey, domain, record)
	}

	err := updateRecord(ctx, zone, accessKey, domain, record)
	return record, err
}

const (
	// The Bunny.net API uses integers to represent record types.
	bunnyTypeA        = 0
	bunnyTypeAAAA     = 1
	bunnyTypeCNAME    = 2
	bunnyTypeTXT      = 3
	bunnyTypeMX       = 4
	bunnyTypeRedirect = 5
	bunnyTypeFlatten  = 6
	bunnyTypePullZone = 7
	bunnyTypeSRV      = 8
	bunnyTypeCAA      = 9
	bunnyTypePTR      = 10
	bunnyTypeScript   = 11
	bunnyTypeNS       = 12
)

// Converts the Bunny.net record type to the libdns record type.
func fromBunnyType(t int) string {
	switch t {
	case bunnyTypeA:
		return "A"
	case bunnyTypeAAAA:
		return "AAAA"
	case bunnyTypeCNAME:
		return "CNAME"
	case bunnyTypeTXT:
		return "TXT"
	case bunnyTypeMX:
		return "MX"
	case bunnyTypeRedirect:
		return "Redirect"
	case bunnyTypeFlatten:
		return "Flatten"
	case bunnyTypePullZone:
		return "PullZone"
	case bunnyTypeSRV:
		return "SRV"
	case bunnyTypeCAA:
		return "CAA"
	case bunnyTypePTR:
		return "PTR"
	case bunnyTypeScript:
		return "Script"
	case bunnyTypeNS:
		return "NS"
	default:
		panic(fmt.Sprintf("unknown record type: %d", t))
	}
}

// Converts the libdns record type to the Bunny.net record type.
func toBunnyType(t string) int {
	switch t {
	case "A":
		return bunnyTypeA
	case "AAAA":
		return bunnyTypeAAAA
	case "CNAME":
		return bunnyTypeCNAME
	case "TXT":
		return bunnyTypeTXT
	case "MX":
		return bunnyTypeMX
	case "Redirect":
		return bunnyTypeRedirect
	case "Flatten":
		return bunnyTypeFlatten
	case "PullZone":
		return bunnyTypePullZone
	case "SRV":
		return bunnyTypeSRV
	case "CAA":
		return bunnyTypeCAA
	case "PTR":
		return bunnyTypePTR
	case "Script":
		return bunnyTypeScript
	case "NS":
		return bunnyTypeNS
	default:
		panic(fmt.Sprintf("unknown record type: %s", t))
	}
}
