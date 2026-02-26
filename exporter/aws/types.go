package aws

const (
	MaxResultsPerPage int32 = 100

	TermOnDemand string = "JRTCKXETXF"
	TermPerHour  string = "6YS6EN2CT7"

	// cpuMemRelation is the CPU-to-memory cost ratio used for normalized cost calculations.
	// CPU-cost = 7.2 * memory-GB-cost
	// https://engineering.empathy.co/cloud-finops-part-4-kubernetes-cost-report/
	CpuMemRelation = 7.2
)

type Pricing struct {
	Product     Product
	ServiceCode string
	Terms       Terms
}

type Terms struct {
	OnDemand map[string]SKU
	Reserved map[string]SKU
}

type Product struct {
	ProductFamily string
	Attributes    map[string]string
	Sku           string
}

type SKU struct {
	PriceDimensions map[string]Details
	Sku             string
	EffectiveDate   string
	OfferTermCode   string
	TermAttributes  string
}

type Details struct {
	Unit         string
	EndRange     string
	Description  string
	AppliesTo    []string
	RateCode     string
	BeginRange   string
	PricePerUnit map[string]string
}

type Instance struct {
	Memory int64
	VCpu   int32
}
