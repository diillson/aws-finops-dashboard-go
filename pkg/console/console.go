package console

import (
	"fmt"
	"github.com/fatih/color"
	"math"
	"strings"

	"github.com/diillson/aws-finops-dashboard-go/internal/shared/types"
	"github.com/pterm/pterm"
)

// Console é uma implementação do ConsoleInterface.
type Console struct{}

// NewConsole cria um novo Console.
func NewConsole() *Console {
	return &Console{}
}

// Print imprime no console.
func (c *Console) Print(a ...interface{}) {
	fmt.Print(a...)
}

// Printf imprime uma string formatada no console.
func (c *Console) Printf(format string, a ...interface{}) {
	fmt.Printf(format, a...)
}

// Println imprime no console com uma nova linha.
func (c *Console) Println(a ...interface{}) {
	fmt.Println(a...)
}

// LogInfo registra uma mensagem de informação.
func (c *Console) LogInfo(format string, a ...interface{}) {
	pterm.Info.Printfln(format, a...)
}

// LogWarning registra uma mensagem de aviso.
func (c *Console) LogWarning(format string, a ...interface{}) {
	pterm.Warning.Printfln(format, a...)
}

// LogError registra uma mensagem de erro.
func (c *Console) LogError(format string, a ...interface{}) {
	pterm.Error.Printfln(format, a...)
}

// LogSuccess registra uma mensagem de sucesso.
func (c *Console) LogSuccess(format string, a ...interface{}) {
	pterm.Success.Printfln(format, a...)
}

// statusHandle é uma implementação do StatusHandle.
type statusHandle struct {
	spinner *pterm.SpinnerPrinter
}

// Status cria um spinner de status com a mensagem especificada.
func (c *Console) Status(message string) types.StatusHandle {
	spinner, _ := pterm.DefaultSpinner.Start(message)
	return &statusHandle{spinner: spinner}
}

// Cores predefinidas para uso consistente
var (
	BrightMagenta = color.New(color.FgMagenta, color.Bold).SprintFunc()
	BoldRed       = color.New(color.FgRed, color.Bold).SprintFunc()
	BrightGreen   = color.New(color.FgGreen, color.Bold).SprintFunc()
	BrightYellow  = color.New(color.FgYellow, color.Bold).SprintFunc()
	BrightRed     = color.New(color.FgRed, color.Bold).SprintFunc()
	BrightCyan    = color.New(color.FgCyan, color.Bold).SprintFunc()
)

// Update atualiza a mensagem de status.
func (h *statusHandle) Update(message string) {
	if h.spinner != nil {
		h.spinner.UpdateText(message)
	}
}

// Stop pára o spinner de status.
func (h *statusHandle) Stop() {
	if h.spinner != nil {
		h.spinner.Stop()
	}
}

// progressHandle é uma implementação do ProgressHandle.
type progressHandle struct {
	bar *pterm.ProgressbarPrinter
}

// Progress cria uma barra de progresso para os itens especificados.
func (c *Console) Progress(items []string) types.ProgressHandle {
	bar, _ := pterm.DefaultProgressbar.WithTotal(len(items)).Start()
	return &progressHandle{bar: bar}
}

func (c *Console) ProgressWithTotal(total int) types.ProgressHandle {
	bar, _ := pterm.DefaultProgressbar.
		WithTotal(total).
		WithTitle("Processing AWS data").
		WithShowElapsedTime(true).
		WithShowCount(true).
		WithRemoveWhenDone(false). // Manter a barra após concluir
		Start()
	return &progressHandle{bar: bar}
}

// Increment incrementa a barra de progresso.
func (h *progressHandle) Increment() {
	if h.bar != nil {
		h.bar.Increment()
	}
}

// Stop pára a barra de progresso.
func (h *progressHandle) Stop() {
	if h.bar != nil {
		h.bar.Stop()
	}
}

// Table é uma implementação do TableInterface.
type Table struct {
	columns []string
	rows    [][]string
}

// CreateTable cria uma nova tabela.
func (c *Console) CreateTable() types.TableInterface {
	return &Table{
		columns: []string{},
		rows:    [][]string{},
	}
}

// AddColumn adiciona uma coluna à tabela.
func (t *Table) AddColumn(name string, options ...interface{}) {
	t.columns = append(t.columns, name)
}

// AddRow adiciona uma linha à tabela.
func (t *Table) AddRow(cells ...interface{}) {
	// Convertemos cada célula para string
	processedCells := make([]string, len(cells))
	for i, cell := range cells {
		processedCells[i] = fmt.Sprint(cell)
	}
	t.rows = append(t.rows, processedCells)
}

// Render renderiza a tabela como uma string.
func (t *Table) Render() string {
	// Use o pterm para criar uma tabela visualmente agradável
	tableData := pterm.TableData{t.columns}
	for _, row := range t.rows {
		tableData = append(tableData, row)
	}

	table := pterm.DefaultTable.
		WithHasHeader().
		WithBoxed().
		WithHeaderStyle(pterm.NewStyle(pterm.FgLightCyan)).
		WithData(tableData)

	renderedTable, _ := table.Srender()
	return renderedTable
}

// DisplayTrendBars exibe gráficos de barras para análise de tendências.
func (c *Console) DisplayTrendBars(monthlyCosts []types.MonthlyCost) {
	// Encontra o valor máximo para escala
	maxCost := 0.0
	for _, cost := range monthlyCosts {
		if cost.Cost > maxCost {
			maxCost = cost.Cost
		}
	}

	if maxCost == 0 {
		pterm.Warning.Println("All costs are $0.00 for this period")
		return
	}

	// Cria um panel com tabela de barras
	tableData := pterm.TableData{
		{"Month", "Cost", "", "MoM Change"},
	}

	var prevCost *float64

	for _, mc := range monthlyCosts {
		// Calcula tamanho da barra
		barLength := int((mc.Cost / maxCost) * 40)
		bar := strings.Repeat("█", barLength)

		// Cores e símbolos padrão
		barColor := pterm.FgBlue.Sprint(bar)
		change := ""

		if prevCost != nil {
			// Calcula mudança percentual mês a mês
			if *prevCost < 0.01 {
				if mc.Cost < 0.01 {
					change = pterm.FgYellow.Sprint("0%")
					barColor = pterm.FgYellow.Sprint(bar)
				} else {
					change = pterm.FgRed.Sprint("N/A")
					barColor = pterm.FgRed.Sprint(bar)
				}
			} else {
				changePercent := ((mc.Cost - *prevCost) / *prevCost) * 100.0

				if math.Abs(changePercent) < 0.01 {
					change = pterm.FgYellow.Sprintf("0%%")
					barColor = pterm.FgYellow.Sprint(bar)
				} else if math.Abs(changePercent) > 999 {
					if changePercent > 0 {
						change = pterm.FgRed.Sprint(">+999%")
						barColor = pterm.FgRed.Sprint(bar)
					} else {
						change = pterm.FgGreen.Sprint(">-999%")
						barColor = pterm.FgGreen.Sprint(bar)
					}
				} else {
					if changePercent > 0 {
						change = pterm.FgRed.Sprintf("+%.2f%%", changePercent)
						barColor = pterm.FgRed.Sprint(bar)
					} else {
						change = pterm.FgGreen.Sprintf("%.2f%%", changePercent)
						barColor = pterm.FgGreen.Sprint(bar)
					}
				}
			}
		}

		tableData = append(tableData, []string{
			mc.Month,
			fmt.Sprintf("$%.2f", mc.Cost),
			barColor,
			change,
		})

		currentCost := mc.Cost
		prevCost = &currentCost
	}

	// Renderiza a tabela
	table := pterm.DefaultTable.WithHasHeader().WithData(tableData)
	renderedTable, _ := table.Srender()

	// Cria um panel azul em volta da tabela
	// Corrigindo o erro com WithTitle em vez de WithTitleTopCenter
	panel := pterm.DefaultBox.WithTitle("AWS Cost Trend Analysis").WithBoxStyle(pterm.NewStyle(pterm.FgCyan)).Sprint(renderedTable)

	fmt.Println("\n" + panel)
}
