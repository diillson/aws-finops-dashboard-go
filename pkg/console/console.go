package console

import (
	"fmt"
	"strings"

	"github.com/diillson/aws-finops-dashboard-go/internal/shared/types"
	"github.com/pterm/pterm"
)

// Console é uma implementação do ConsoleInterface.
type Console struct{}

// NewConsole cria um novo Console.
func NewConsole() types.ConsoleInterface {
	return &Console{}
}

// Print, Printf, Println
func (c *Console) Print(a ...interface{})                 { fmt.Print(a...) }
func (c *Console) Printf(format string, a ...interface{}) { fmt.Printf(format, a...) }
func (c *Console) Println(a ...interface{})               { fmt.Println(a...) }

// Loggers
func (c *Console) LogInfo(format string, a ...interface{})    { pterm.Info.Printfln(format, a...) }
func (c *Console) LogWarning(format string, a ...interface{}) { pterm.Warning.Printfln(format, a...) }
func (c *Console) LogError(format string, a ...interface{})   { pterm.Error.Printfln(format, a...) }
func (c *Console) LogSuccess(format string, a ...interface{}) { pterm.Success.Printfln(format, a...) }

// --- Status (Spinner) ---
type statusHandle struct{ spinner *pterm.SpinnerPrinter }

func (c *Console) Status(message string) types.StatusHandle {
	spinner, _ := pterm.DefaultSpinner.Start(message)
	return &statusHandle{spinner: spinner}
}
func (h *statusHandle) Update(message string) { h.spinner.UpdateText(message) }
func (h *statusHandle) Stop()                 { h.spinner.Stop() }

// --- Progress Bar & Multi Printer ---

// GetMultiPrinter retorna a instância padrão do MultiPrinter da pterm.
func (c *Console) GetMultiPrinter() *pterm.MultiPrinter {
	return &pterm.DefaultMultiPrinter
}

// NewProgressbar cria e retorna uma nova barra de progresso da pterm, mas NÃO a inicia.
func (c *Console) NewProgressbar(total int, title string) *pterm.ProgressbarPrinter {
	return pterm.DefaultProgressbar.WithTotal(total).WithTitle(title).WithRemoveWhenDone(true)
}

// --- Table ---
type Table struct {
	columns []string
	rows    [][]string
}

func (c *Console) CreateTable() types.TableInterface {
	return &Table{}
}
func (t *Table) AddColumn(name string, options ...interface{}) { t.columns = append(t.columns, name) }
func (t *Table) AddRow(cells ...interface{}) {
	row := make([]string, len(cells))
	for i, cell := range cells {
		row[i] = fmt.Sprint(cell)
	}
	t.rows = append(t.rows, row)
}
func (t *Table) Render() string {
	tableData := pterm.TableData{t.columns}
	for _, row := range t.rows {
		tableData = append(tableData, row)
	}
	s, _ := pterm.DefaultTable.WithHasHeader().WithBoxed().WithData(tableData).Srender()
	return s
}

// --- Trend Bars ---
func (c *Console) DisplayTrendBars(monthlyCosts []types.MonthlyCost) {
	if len(monthlyCosts) == 0 {
		pterm.Warning.Println("No trend data to display.")
		return
	}
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

	tableData := pterm.TableData{{"Month", "Cost", "", "MoM Change"}}
	var prevCost *float64

	for _, mc := range monthlyCosts {
		barLength := int((mc.Cost / maxCost) * 40)
		if barLength < 0 {
			barLength = 0
		}
		bar := strings.Repeat("█", barLength)
		barColor := pterm.FgBlue
		change := ""

		if prevCost != nil {
			if *prevCost > 0.01 {
				changePercent := ((mc.Cost - *prevCost) / *prevCost) * 100.0
				if changePercent > 0.01 {
					change = pterm.FgRed.Sprintf("+%.2f%%", changePercent)
					barColor = pterm.FgRed
				} else if changePercent < -0.01 {
					change = pterm.FgGreen.Sprintf("%.2f%%", changePercent)
					barColor = pterm.FgGreen
				} else {
					change = pterm.FgYellow.Sprint("0.00%")
					barColor = pterm.FgYellow
				}
			}
		}
		tableData = append(tableData, []string{mc.Month, fmt.Sprintf("$%.2f", mc.Cost), barColor.Sprint(bar), change})
		currentCost := mc.Cost
		prevCost = &currentCost
	}

	table, _ := pterm.DefaultTable.WithHasHeader().WithData(tableData).Srender()
	panel := pterm.DefaultBox.WithTitle("AWS Cost Trend Analysis").Sprint(table)
	fmt.Println("\n" + panel)
}
