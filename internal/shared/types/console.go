package types

// ConsoleInterface define a interface para saída no console.
type ConsoleInterface interface {
	Print(a ...interface{})
	Printf(format string, a ...interface{})
	Println(a ...interface{})

	LogInfo(format string, a ...interface{})
	LogWarning(format string, a ...interface{})
	LogError(format string, a ...interface{})
	LogSuccess(format string, a ...interface{})

	Status(message string) StatusHandle
	Progress(items []string) ProgressHandle

	CreateTable() TableInterface
	DisplayTrendBars(monthlyCosts []MonthlyCost)

	ProgressWithTotal(total int) ProgressHandle
}

// StatusHandle é uma interface para atualizar uma mensagem de status.
type StatusHandle interface {
	Update(message string)
	Stop()
}

// ProgressHandle é uma interface para atualizar uma barra de progresso.
type ProgressHandle interface {
	Increment()
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
