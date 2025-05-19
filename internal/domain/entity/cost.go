package entity

import "time"

// ServiceCost represents a cost amount for a specific AWS service.
type ServiceCost struct {
	ServiceName string  `json:"service_name"`
	Cost        float64 `json:"cost"`
}

// CostData contains all cost-related information for an AWS account.
type CostData struct {
	AccountID                 string        `json:"account_id,omitempty"`
	CurrentMonthCost          float64       `json:"current_month"`
	LastMonthCost             float64       `json:"last_month"`
	CurrentMonthCostByService []ServiceCost `json:"current_month_cost_by_service"`
	Budgets                   []BudgetInfo  `json:"budgets"`
	CurrentPeriodName         string        `json:"current_period_name"`
	PreviousPeriodName        string        `json:"previous_period_name"`
	TimeRange                 int           `json:"time_range,omitempty"`
	CurrentPeriodStart        time.Time     `json:"current_period_start"`
	CurrentPeriodEnd          time.Time     `json:"current_period_end"`
	PreviousPeriodStart       time.Time     `json:"previous_period_start"`
	PreviousPeriodEnd         time.Time     `json:"previous_period_end"`
	MonthlyCosts              []MonthlyCost `json:"monthly_costs,omitempty"`
}

// MonthlyCost represents the cost for a specific month, used for trend analysis.
type MonthlyCost struct {
	Month string  `json:"month"`
	Cost  float64 `json:"cost"`
}
