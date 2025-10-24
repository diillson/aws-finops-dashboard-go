package entity

import "time"

// ServiceCoverage representa a cobertura por serviço (% e custos relacionados).
type ServiceCoverage struct {
	Service         string  `json:"service"`
	CoveragePercent float64 `json:"coverage_percent"`
	CoveredCost     float64 `json:"covered_cost,omitempty"`
	OnDemandCost    float64 `json:"on_demand_cost,omitempty"`
}

// SPSummary agrega os principais números de Savings Plans.
type SPSummary struct {
	AccountID          string            `json:"account_id"`
	CoveragePercent    float64           `json:"coverage_percent"`
	UtilizationPercent float64           `json:"utilization_percent"`
	TotalCommitment    float64           `json:"total_commitment,omitempty"`
	UsedCommitment     float64           `json:"used_commitment,omitempty"`
	UnusedCommitment   float64           `json:"unused_commitment,omitempty"`
	SavingsAmount      float64           `json:"savings_amount,omitempty"`
	PerServiceCoverage []ServiceCoverage `json:"per_service_coverage,omitempty"`
	PeriodStart        time.Time         `json:"period_start"`
	PeriodEnd          time.Time         `json:"period_end"`
	PeriodName         string            `json:"period_name"`
	DataUnavailable    bool              `json:"data_unavailable,omitempty"` // <-- NOVO
}

// RISummary agrega os principais números de Reserved Instances.
type RISummary struct {
	AccountID          string            `json:"account_id"`
	CoveragePercent    float64           `json:"coverage_percent"`
	UtilizationPercent float64           `json:"utilization_percent"`
	UsedHours          float64           `json:"used_hours,omitempty"`
	UnusedHours        float64           `json:"unused_hours,omitempty"`
	TotalReservedHours float64           `json:"total_reserved_hours,omitempty"`
	PerServiceCoverage []ServiceCoverage `json:"per_service_coverage,omitempty"`
	PeriodStart        time.Time         `json:"period_start"`
	PeriodEnd          time.Time         `json:"period_end"`
	PeriodName         string            `json:"period_name"`
	DataUnavailable    bool              `json:"data_unavailable,omitempty"` // <-- NOVO
}

// CommitmentsReport combina SP e RI para uma conta/perfil.
type CommitmentsReport struct {
	AccountID  string    `json:"account_id"`
	Profile    string    `json:"profile"`
	SPSummary  SPSummary `json:"sp_summary"`
	RISummary  RISummary `json:"ri_summary"`
	PeriodName string    `json:"period_name"`
}
