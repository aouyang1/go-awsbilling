package main

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cespare/xxhash"
)

var (
	logger     = log.New(os.Stdout, "", log.Ldate|log.Lshortfile)
	timeLayout = "2006-01-02T15:04:05Z"
)

type LineItemID struct {
	UID      uint64
	Start    time.Time
	End      time.Time
	Bill     *Bill
	LineItem *LineItem
}

func NewLineItemID(id, timeInterval string) (*LineItemID, error) {
	l := new(LineItemID)
	l.UID = xxhash.Sum64String(id)
	timeIntStr := strings.Split(timeInterval, "/")
	if len(timeIntStr) != 2 {
		return nil, fmt.Errorf("Invalid time interval, %s", timeInterval)
	}

	var err error
	l.Start, err = time.Parse(timeLayout, timeIntStr[0])
	if err != nil {
		return nil, fmt.Errorf("Could not parse start interval, %v", err)
	}
	l.End, err = time.Parse(timeLayout, timeIntStr[1])
	if err != nil {
		return nil, fmt.Errorf("Coult not parse end interval, %v", err)
	}
	return l, nil
}

type Identity struct {
	LineItemIDs map[time.Time][]*LineItemID // map of start timestamps to a slice of LineItemIDs
	TimePts     []time.Time                 // sorted order of start timestamps with identity
}

func NewIdentity() *Identity {
	return &Identity{LineItemIDs: make(map[time.Time][]*LineItemID)}
}

func (i *Identity) AddLineItemID(l *LineItemID) {
	lids, exists := i.LineItemIDs[l.Start]
	if exists {
		for _, lid := range lids {
			if lid.UID == l.UID {
				logger.Printf("LineItemID, %s, already exists in Identity\n", l.UID)
				return
			}
		}
		lids = append(lids, l)
		return
	}

	i.LineItemIDs[l.Start] = []*LineItemID{l}

	// linearly scan to find where to put timepoint in sorted order
	// most cases should be at the end
	for j := len(i.TimePts) - 1; j >= 0; j-- {
		if l.Start.After(i.TimePts[j]) {
			// timestamp is after latest timestamp
			if j == len(i.TimePts)-1 {
				i.TimePts = append(i.TimePts, l.Start)
				return
			}

			// timestamp is somewhere in between
			i.TimePts = append(i.TimePts, time.Time{})
			copy(i.TimePts[j+2:], i.TimePts[j+1:])
			i.TimePts[j+1] = l.Start
			return
		}
	}

	// timepoint is earlier than all timepoints
	i.TimePts = append(i.TimePts, time.Time{})
	copy(i.TimePts[1:], i.TimePts[:])
	i.TimePts[0] = l.Start
	return
}

type LineItem struct {
	AvailabilityZone    string
	BlendedCost         float64
	BlendedRate         float64
	CurrencyCode        string
	LegalEntity         string
	LineItemDescription string
	LineItemType        string
	NormalizationFactor float64
	Operation           string
	ProductCode         string
	ResourceID          string
	TaxType             string
	UnblendedCost       float64
	UnblendedRate       float64
	UsageAccountID      string
	UsageAmount         int
	UsageEndDate        time.Time
	UsageStartDate      time.Time
	UsageType           string
}

func NewLineItem(az, blendedCost, blendedRate, currencyCode, legalEntity, lineItemDescription, lineItemType,
	normalizationFactor, operation, productCode, resourceID, taxType, unblendedCost, unblendedRate,
	usageAccountID, usageAmount, start, end, usageType string) (*LineItem, error) {
	l := new(LineItem)

	var err error

	l.BlendedCost, err = strconv.ParseFloat(blendedCost, 64)
	if err != nil {
		return nil, fmt.Errorf("Could not parse blendedCost, %v", err)
	}

	l.BlendedRate, err = strconv.ParseFloat(blendedRate, 64)
	if err != nil {
		return nil, fmt.Errorf("Could not parse blendedRate, %v", err)
	}

	l.NormalizationFactor, err = strconv.ParseFloat(normalizationFactor, 64)
	if err != nil && normalizationFactor != "" {
		return nil, fmt.Errorf("Could not parse normalizationFactor, %v", err)
	}

	l.UnblendedCost, err = strconv.ParseFloat(unblendedCost, 64)
	if err != nil {
		return nil, fmt.Errorf("Could not parse unblendedCost, %v", err)
	}

	l.UnblendedRate, err = strconv.ParseFloat(unblendedRate, 64)
	if err != nil && unblendedRate != "" {
		return nil, fmt.Errorf("Could not parse unblendedRate, %v", err)
	}

	l.UsageStartDate, err = time.Parse(timeLayout, start)
	if err != nil {
		return nil, fmt.Errorf("Could not parse start interval, %v", err)
	}
	l.UsageEndDate, err = time.Parse(timeLayout, end)
	if err != nil {
		return nil, fmt.Errorf("Coult not parse end interval, %v", err)
	}

	l.CurrencyCode = currencyCode
	l.LegalEntity = legalEntity
	l.LineItemDescription = lineItemDescription
	l.LineItemType = lineItemType
	l.Operation = operation
	l.ProductCode = productCode
	l.ResourceID = resourceID
	l.TaxType = taxType

	return l, nil
}

type Bill struct {
	BillingEntity          string
	BillType               string
	InvoiceID              string
	PayerAccountID         uint64
	BillingPeriodEndDate   time.Time
	BillingPeriodStartDate time.Time
}

func NewBill(entity, billType, invoiceID, payerAccountID, start, end string) (*Bill, error) {
	b := new(Bill)

	var err error
	b.PayerAccountID, err = strconv.ParseUint(payerAccountID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("Unable to parse PayerAccountID, %s, %v", payerAccountID, err)
	}

	b.BillingPeriodStartDate, err = time.Parse(timeLayout, start)
	if err != nil {
		return nil, fmt.Errorf("Could not parse start interval, %v", err)
	}
	b.BillingPeriodEndDate, err = time.Parse(timeLayout, end)
	if err != nil {
		return nil, fmt.Errorf("Coult not parse end interval, %v", err)
	}

	b.BillingEntity = entity
	b.BillType = billType
	b.InvoiceID = invoiceID

	return b, nil
}

func main() {
	filename := "/Users/aouyang/Downloads/ao-aws-1.csv.gz"
	file, err := os.Open(filename)

	if err != nil {
		logger.Fatal(err)
	}

	gz, err := gzip.NewReader(file)
	if err != nil {
		logger.Fatal(err)
	}

	scanner := bufio.NewScanner(gz)

	scanner.Scan()
	headers := strings.Split(scanner.Text(), ",")
	headerIdx := make(map[string]int)
	for i, header := range headers {
		headerIdx[header] = i
	}

	identity := NewIdentity()
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), ",")
		l, err := NewLineItemID(
			parts[headerIdx["identity/LineItemId"]],
			parts[headerIdx["identity/TimeInterval"]],
		)
		if err != nil {
			logger.Fatal(err)
		}
		l.Bill, err = NewBill(
			parts[headerIdx["bill/Entity"]],
			parts[headerIdx["bill/BillType"]],
			parts[headerIdx["bill/InvoiceId"]],
			parts[headerIdx["bill/PayerAccountId"]],
			parts[headerIdx["bill/BillingPeriodStartDate"]],
			parts[headerIdx["bill/BillingPeriodEndDate"]],
		)
		if err != nil {
			logger.Fatal(err)
		}
		l.LineItem, err = NewLineItem(
			parts[headerIdx["lineItem/AvailabilityZone"]],
			parts[headerIdx["lineItem/BlendedCost"]],
			parts[headerIdx["lineItem/BlendedCost"]],
			parts[headerIdx["lineItem/CurrencyCode"]],
			parts[headerIdx["lineItem/LegalEntity"]],
			parts[headerIdx["lineItem/LineItemDescription"]],
			parts[headerIdx["lineItem/LineItemType"]],
			parts[headerIdx["lineItem/NormalizationFactor"]],
			parts[headerIdx["lineItem/Operation"]],
			parts[headerIdx["lineItem/ProductCode"]],
			parts[headerIdx["lineItem/ResourceId"]],
			parts[headerIdx["lineItem/TaxType"]],
			parts[headerIdx["lineItem/UnblendedCost"]],
			parts[headerIdx["lineItem/UnblendedRate"]],
			parts[headerIdx["lineItem/UsageAccountId"]],
			parts[headerIdx["lineItem/UsageAmount"]],
			parts[headerIdx["lineItem/UsageStartDate"]],
			parts[headerIdx["lineItem/UsageEndDate"]],
			parts[headerIdx["lineItem/UsageType"]],
		)
		if err != nil {
			logger.Fatal(err)
		}
		identity.AddLineItemID(l)
	}

	logger.Println(identity)
	// catch any issues while trying to close the file and gzip reader
	err = file.Close()
	if gzerr := gz.Close(); err == nil {
		err = gzerr
	}
	if err != nil {
		logger.Fatal(err)
	}

}
