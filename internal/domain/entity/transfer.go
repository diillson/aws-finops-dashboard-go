package entity

import "time"

// DataTransferCategoryCost representa o custo agregado por categoria de transferência.
type DataTransferCategoryCost struct {
	Category string  `json:"category"`
	Cost     float64 `json:"cost"`
}

// DataTransferLine representa uma linha individual (service + usage type) com custo.
type DataTransferLine struct {
	Service   string  `json:"service"`
	UsageType string  `json:"usage_type"`
	Cost      float64 `json:"cost"`
}

// DataTransferReport é o relatório completo para um perfil/conta.
type DataTransferReport struct {
	AccountID  string                     `json:"account_id"`
	Total      float64                    `json:"total"`
	Categories []DataTransferCategoryCost `json:"categories"`
	TopLines   []DataTransferLine         `json:"top_lines"`

	// Metadados de período (úteis para export/render)
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
	PeriodName  string    `json:"period_name"`
}
