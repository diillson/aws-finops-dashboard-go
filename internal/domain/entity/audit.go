package entity

// AuditData represents the audit information for a specific AWS profile.
type AuditData struct {
	Profile           string `json:"profile"`
	AccountID         string `json:"account_id"`
	UntaggedResources string `json:"untagged_resources"`
	StoppedInstances  string `json:"stopped_instances"`
	UnusedVolumes     string `json:"unused_volumes"`
	UnusedEIPs        string `json:"unused_eips"`
	BudgetAlerts      string `json:"budget_alerts"`
}
