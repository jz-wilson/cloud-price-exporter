package provider

import "regexp"

// ScrapeResult is the universal metric record produced by all cloud providers.
type ScrapeResult struct {
	Name               string
	Value              float64
	Region             string
	AvailabilityZone   string
	InstanceType       string
	InstanceLifecycle  string
	ProductDescription string
	OperatingSystem    string
	SavingPlanOption   string
	SavingPlanDuration int
	SavingPlanType     string
	Memory             string
	VCpu               string
}

// Contains reports whether v is present in elems.
func Contains(elems []string, v string) bool {
	for _, s := range elems {
		if v == s {
			return true
		}
	}
	return false
}

// IsMatchAny reports whether text matches any of the regular expressions.
func IsMatchAny(regexList []*regexp.Regexp, text string) bool {
	for _, regex := range regexList {
		if regex.MatchString(text) {
			return true
		}
	}
	return false
}
