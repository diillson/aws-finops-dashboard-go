package entity

// ProfileData represents all data collected for a specific AWS profile or a combined group.
type ProfileData struct {
	// Profile é o identificador do grupo (ex: "profile-1" ou "dev, staging, prod").
	Profile string `json:"profile"`

	// AccountID é o ID da conta AWS associada.
	AccountID string `json:"account_id"`

	// LastMonth é o custo total do período anterior.
	LastMonth float64 `json:"last_month"`

	// CurrentMonth é o custo total do período atual.
	CurrentMonth float64 `json:"current_month"`

	// ServiceCosts contém os dados brutos de custo por serviço.
	ServiceCosts []ServiceCost `json:"service_costs"`

	// ServiceCostsFormatted é uma lista de strings prontas para exibição na UI.
	ServiceCostsFormatted []string `json:"-"` // Omitido do JSON por ser um dado de apresentação

	// BudgetInfo é uma lista de strings formatadas sobre orçamentos para a UI.
	BudgetInfo []string `json:"-"` // Omitido do JSON

	// EC2Summary contém os dados brutos de contagem de instâncias por estado.
	EC2Summary EC2Summary `json:"ec2_summary"`

	// EC2SummaryFormatted é uma lista de strings formatadas sobre instâncias EC2 para a UI.
	EC2SummaryFormatted []string `json:"-"` // Omitido do JSON

	// Success indica se o processamento para este perfil/grupo foi bem-sucedido.
	Success bool `json:"success"`

	// Err armazena qualquer erro que tenha ocorrido durante o processamento.
	// Usar 'error' permite um tratamento de erros mais robusto.
	Err error `json:"-"` // Omitido do JSON para não expor detalhes de implementação

	// CurrentPeriodName é o nome descritivo do período de custo atual (ex: "Current 30 days cost").
	CurrentPeriodName string `json:"current_period_name"`

	// PreviousPeriodName é o nome descritivo do período de custo anterior.
	PreviousPeriodName string `json:"previous_period_name"`

	// PercentChangeInCost armazena a variação percentual do custo entre os períodos.
	PercentChangeInCost *float64 `json:"percent_change_in_total_cost,omitempty"`
}
