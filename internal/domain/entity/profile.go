package entity

// ProfileData represents all data collected for a specific AWS profile.
type ProfileData struct {
	Profile               string        `json:"profile"`
	AccountID             string        `json:"account_id"`
	LastMonth             float64       `json:"last_month"`
	CurrentMonth          float64       `json:"current_month"`
	ServiceCosts          []ServiceCost `json:"service_costs"`
	ServiceCostsFormatted []string      `json:"service_costs_formatted"`
	BudgetInfo            []string      `json:"budget_info"`
	EC2Summary            EC2Summary    `json:"ec2_summary"`
	EC2SummaryFormatted   []string      `json:"ec2_summary_formatted"`
	Success               bool          `json:"success"`
	Error                 string        `json:"error,omitempty"`
	CurrentPeriodName     string        `json:"current_period_name"`
	PreviousPeriodName    string        `json:"previous_period_name"`
	PercentChangeInCost   *float64      `json:"percent_change_in_total_cost,omitempty"`
}
