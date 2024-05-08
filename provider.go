package bunny

import (
	"context"
	"fmt"
	"strings"

	"github.com/libdns/libdns"
)

// Provider facilitates DNS record manipulation with Bunny.net
type Provider struct {
	// AccessKey is the Bunny.net API key - see https://docs.bunny.net/reference/bunnynet-api-overview
	AccessKey string `json:"access_key"`
	Zone      string `json:"zone"`
}

// GetRecords lists all the records in the zone.
func (p *Provider) GetRecords(ctx context.Context, domain string) ([]libdns.Record, error) {
	records, err := getAllRecords(ctx, p.Zone, p.AccessKey, unFQDN(domain))
	if err != nil {
		return nil, err
	}

	return records, nil
}

// AppendRecords adds records to the zone. It returns the records that were added.
func (p *Provider) AppendRecords(ctx context.Context, domain string, records []libdns.Record) ([]libdns.Record, error) {
	var appendedRecords []libdns.Record

	for _, record := range records {
		newRecord, err := createRecord(ctx, p.Zone, p.AccessKey, unFQDN(domain), record)
		if err != nil {
			return nil, err
		}
		appendedRecords = append(appendedRecords, newRecord)
	}

	return appendedRecords, nil
}

// SetRecords sets the records in the zone, either by updating existing records or creating new ones.
// It returns the updated records.
func (p *Provider) SetRecords(ctx context.Context, domain string, records []libdns.Record) ([]libdns.Record, error) {
	var setRecords []libdns.Record

	for _, record := range records {
		setRecord, err := createOrUpdateRecord(ctx, p.Zone, p.AccessKey, unFQDN(domain), record)
		if err != nil {
			return setRecords, err
		}
		setRecords = append(setRecords, setRecord)
	}

	return setRecords, nil
}

// DeleteRecords deletes the records from the zone. It returns the records that were deleted.
func (p *Provider) DeleteRecords(ctx context.Context, domain string, records []libdns.Record) ([]libdns.Record, error) {
	var deletedRecords []libdns.Record

	for _, record := range records {
		err := deleteRecord(ctx, p.Zone, p.AccessKey, unFQDN(domain), record)
		if err != nil {
			fmt.Println(err)
		} else {
			deletedRecords = append(deletedRecords, record)
		}
	}

	return deletedRecords, nil
}

// unFQDN trims any trailing "." from fqdn. Bunny.net's API does not use FQDNs.
func unFQDN(fqdn string) string {
	return strings.TrimSuffix(fqdn, ".")
}

// Interface guards
var (
	_ libdns.RecordGetter   = (*Provider)(nil)
	_ libdns.RecordAppender = (*Provider)(nil)
	_ libdns.RecordSetter   = (*Provider)(nil)
	_ libdns.RecordDeleter  = (*Provider)(nil)
)
