package types

import "github.com/pterm/pterm"

// ConsoleInterface define a interface para saída no console.
type ConsoleInterface interface {
	Print(a ...interface{})
	Printf(format string, a ...interface{})
	Println(a ...interface{})

	LogInfo(format string, a ...interface{})
	LogWarning(format string, a ...interface{})
	LogError(format string, a ...interface{})
	LogSuccess(format string, a ...interface{})

	// Status retorna um manipulador para um spinner.
	Status(message string) StatusHandle

	// GetMultiPrinter retorna a instância padrão do MultiPrinter da pterm.
	GetMultiPrinter() *pterm.MultiPrinter

	// NewProgressbar cria e retorna uma nova barra de progresso da pterm.
	NewProgressbar(total int, title string) *pterm.ProgressbarPrinter

	// CreateTable cria uma interface para uma tabela.
	CreateTable() TableInterface

	// DisplayTrendBars exibe os gráficos de barras de tendência.
	DisplayTrendBars(monthlyCosts []MonthlyCost)
}

// StatusHandle é uma interface para atualizar uma mensagem de status (spinner).
type StatusHandle interface {
	Update(message string)
	Stop()
}

// TableInterface define a interface para criar e manipular tabelas.
type TableInterface interface {
	AddColumn(name string, options ...interface{})
	AddRow(cells ...interface{})
	Render() string
}

// MonthlyCost representa o custo para um mês específico, usado para gráficos de tendência.
type MonthlyCost struct {
	Month string  `json:"month"`
	Cost  float64 `json:"cost"`
}
