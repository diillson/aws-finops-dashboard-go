package entity

// BudgetInfo represents a budget with actual and forecasted spend.
type BudgetInfo struct {
	Name     string  `json:"name"`
	Limit    float64 `json:"limit"`
	Actual   float64 `json:"actual"`
	Forecast float64 `json:"forecast,omitempty"`
}
