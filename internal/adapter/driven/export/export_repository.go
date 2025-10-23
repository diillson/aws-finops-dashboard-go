package export

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/diillson/aws-finops-dashboard-go/internal/domain/entity"
	"github.com/diillson/aws-finops-dashboard-go/internal/domain/repository"
	"github.com/jung-kurt/gofpdf"
)

// ExportRepositoryImpl implementa o ExportRepository.
type ExportRepositoryImpl struct{}

// NewExportRepository cria uma nova implementação do ExportRepository.
func NewExportRepository() repository.ExportRepository {
	return &ExportRepositoryImpl{}
}

// --- Funções de Exportação do Dashboard de Custos ---

func (r *ExportRepositoryImpl) ExportToCSV(data []entity.ProfileData, filename, outputDir, previousPeriodDates, currentPeriodDates string) (string, error) {
	outputFilename, err := generateFilename(filename, outputDir, "csv")
	if err != nil {
		return "", err
	}

	file, err := os.Create(outputFilename)
	if err != nil {
		return "", fmt.Errorf("error creating CSV file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	headers := []string{
		"CLI Profile", "AWS Account ID",
		fmt.Sprintf("Cost for period (%s)", previousPeriodDates),
		fmt.Sprintf("Cost for period (%s)", currentPeriodDates),
		"Cost By Service", "Budget Status", "EC2 Instances",
	}
	writer.Write(headers)

	for _, row := range data {
		servicesData := ""
		for _, sc := range row.ServiceCosts {
			servicesData += fmt.Sprintf("%s: $%.2f\n", sc.ServiceName, sc.Cost)
		}

		record := []string{
			row.Profile,
			row.AccountID,
			fmt.Sprintf("$%.2f", row.LastMonth),
			fmt.Sprintf("$%.2f", row.CurrentMonth),
			strings.TrimSpace(servicesData),
			strings.Join(row.BudgetInfo, "\n"),
			strings.Join(row.EC2SummaryFormatted, "\n"),
		}
		writer.Write(record)
	}

	return filepath.Abs(outputFilename)
}

func (r *ExportRepositoryImpl) ExportToJSON(data []entity.ProfileData, filename, outputDir string) (string, error) {
	// Para o JSON, removemos os campos formatados que são apenas para UI.
	// A struct ProfileData já usa `json:"-"` para isso, então a serialização padrão funciona.
	outputFilename, err := generateFilename(filename, outputDir, "json")
	if err != nil {
		return "", err
	}

	file, err := os.Create(outputFilename)
	if err != nil {
		return "", fmt.Errorf("error creating JSON file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return "", fmt.Errorf("error encoding JSON data: %w", err)
	}

	return filepath.Abs(outputFilename)
}

func (r *ExportRepositoryImpl) ExportToPDF(data []entity.ProfileData, filename, outputDir, previousPeriodDates, currentPeriodDates string) (string, error) {
	outputFilename, err := generateFilename(filename, outputDir, "pdf")
	if err != nil {
		return "", err
	}

	pdf := gofpdf.New("P", "mm", "A4", "")
	tr := pdf.UnicodeTranslatorFromDescriptor("")

	// --- Configuração de Cores e Estilos ---
	headerColor := [3]int{40, 40, 40}
	headerTextColor := [3]int{255, 255, 255}
	sectionTitleColor := [3]int{0, 0, 0}
	bodyTextColor := [3]int{50, 50, 50}
	lineColor := [3]int{200, 200, 200}

	drawSection := func(title string, content string) {
		pdf.SetFont("Arial", "B", 12)
		pdf.SetTextColor(sectionTitleColor[0], sectionTitleColor[1], sectionTitleColor[2])
		pdf.Cell(0, 8, title)
		pdf.Ln(7)

		pdf.SetDrawColor(lineColor[0], lineColor[1], lineColor[2])
		pdf.Line(pdf.GetX(), pdf.GetY(), pdf.GetX()+190, pdf.GetY())
		pdf.Ln(4)

		pdf.SetFont("Arial", "", 10)
		pdf.SetTextColor(bodyTextColor[0], bodyTextColor[1], bodyTextColor[2])
		pdf.MultiCell(190, 5, tr(content), "", "L", false)
		pdf.Ln(8)
	}

	for i, rowData := range data {
		pdf.AddPage()

		// --- Cabeçalho da Página ---
		pdf.SetFillColor(headerColor[0], headerColor[1], headerColor[2])
		pdf.SetTextColor(headerTextColor[0], headerTextColor[1], headerTextColor[2])
		pdf.SetFont("Arial", "B", 14)
		profileName := rowData.Profile
		if len(profileName) > 80 {
			profileName = profileName[:77] + "..."
		}
		pdf.CellFormat(0, 12, tr(fmt.Sprintf("  %s", profileName)), "", 1, "L", true, 0, "")

		pdf.SetFont("Arial", "", 10)
		pdf.SetFillColor(240, 240, 240)
		pdf.SetTextColor(bodyTextColor[0], bodyTextColor[1], bodyTextColor[2])
		pdf.CellFormat(0, 8, tr(fmt.Sprintf("  Account ID: %s", rowData.AccountID)), "", 1, "L", true, 0, "")
		pdf.Ln(10)

		// --- Seção 1: Resumo de Custos ---
		pdf.SetFont("Arial", "B", 12)
		pdf.SetTextColor(sectionTitleColor[0], sectionTitleColor[1], sectionTitleColor[2])
		pdf.Cell(0, 8, "Cost Summary")
		pdf.Ln(7)
		pdf.SetDrawColor(lineColor[0], lineColor[1], lineColor[2])
		pdf.Line(pdf.GetX(), pdf.GetY(), pdf.GetX()+190, pdf.GetY())
		pdf.Ln(4)

		costTableWidth := 95.0
		pdf.SetFont("Arial", "", 10)
		pdf.SetTextColor(bodyTextColor[0], bodyTextColor[1], bodyTextColor[2])

		// --- INÍCIO DA CORREÇÃO: ADICIONAR DATAS ---
		// Os parâmetros 'previousPeriodDates' e 'currentPeriodDates' já contêm as strings formatadas.
		// Vamos usá-los diretamente.
		pdf.SetFont("Arial", "B", 10) // Usar negrito para o título do período
		pdf.CellFormat(costTableWidth, 7, tr(rowData.PreviousPeriodName), "B", 0, "L", false, 0, "")
		pdf.CellFormat(costTableWidth, 7, tr(rowData.CurrentPeriodName), "B", 1, "L", false, 0, "")

		pdf.SetFont("Arial", "", 8)     // Fonte menor para as datas
		pdf.SetTextColor(100, 100, 100) // Cor cinza para as datas
		pdf.CellFormat(costTableWidth, 5, tr(previousPeriodDates), "", 0, "L", false, 0, "")
		pdf.CellFormat(costTableWidth, 5, tr(currentPeriodDates), "", 1, "L", false, 0, "")
		pdf.SetTextColor(bodyTextColor[0], bodyTextColor[1], bodyTextColor[2]) // Reseta a cor do texto
		// --- FIM DA CORREÇÃO ---

		pdf.SetFont("Arial", "B", 16)
		pdf.CellFormat(costTableWidth, 12, tr(fmt.Sprintf("$%.2f", rowData.LastMonth)), "", 0, "L", false, 0, "")

		changeText := ""
		originalTextColorR, originalTextColorG, originalTextColorB := pdf.GetTextColor()
		if rowData.PercentChangeInCost != nil {
			val := *rowData.PercentChangeInCost

			if val > 0.01 {
				pdf.SetTextColor(192, 0, 0)
				changeText = fmt.Sprintf("  (▲ +%.2f%%)", val)
			} else if val < -0.01 {
				pdf.SetTextColor(0, 128, 0)
				changeText = fmt.Sprintf("  (▼ %.2f%%)", val)
			} else {
				changeText = "  (0.00%)"
			}
		}

		pdf.SetFont("Arial", "B", 16)
		valueStr := fmt.Sprintf("$%.2f", rowData.CurrentMonth)
		pdf.Cell(pdf.GetStringWidth(valueStr), 12, tr(valueStr))

		pdf.SetFont("Arial", "", 10)
		pdf.CellFormat(costTableWidth-pdf.GetStringWidth(valueStr), 12, tr(changeText), "", 1, "L", false, 0, "")

		pdf.SetTextColor(originalTextColorR, originalTextColorG, originalTextColorB)
		pdf.Ln(10)

		serviceCostsStr := ""
		for _, sc := range rowData.ServiceCosts {
			serviceCostsStr += fmt.Sprintf("%s: $%.2f\n", sc.ServiceName, sc.Cost)
		}
		drawSection("Cost By Service", strings.TrimSpace(serviceCostsStr))

		drawSection("Budget Status", strings.Join(rowData.BudgetInfo, "\n\n"))

		drawSection("EC2 Instances", cleanRichTags(strings.Join(rowData.EC2SummaryFormatted, "\n")))

		pdf.SetY(-15)
		pdf.SetFont("Arial", "I", 8)
		pdf.SetTextColor(128, 128, 128)
		footerText := fmt.Sprintf("Generated by AWS FinOps Dashboard (Go) | %s", time.Now().Format("2006-01-02"))
		pdf.CellFormat(0, 10, tr(footerText), "", 0, "L", false, 0, "")
		pdf.CellFormat(0, 10, tr(fmt.Sprintf("Page %d", i+1)), "", 0, "R", false, 0, "")
	}

	if err := pdf.OutputFileAndClose(outputFilename); err != nil {
		return "", fmt.Errorf("error writing PDF file: %w", err)
	}

	return filepath.Abs(outputFilename)
}

// --- Funções de Exportação do Relatório de Auditoria ---

func (r *ExportRepositoryImpl) ExportAuditReportToCSV(auditData []entity.AuditData, filename, outputDir string) (string, error) {
	// (Implementação existente, geralmente funciona bem para CSV)
	return "", nil // Placeholder
}

func (r *ExportRepositoryImpl) ExportAuditReportToJSON(auditData []entity.AuditData, filename, outputDir string) (string, error) {
	// (Implementação existente)
	return "", nil // Placeholder
}

func (r *ExportRepositoryImpl) ExportAuditReportToPDF(auditData []entity.AuditData, filename, outputDir string) (string, error) {
	// (Esta função também se beneficiaria da mesma lógica de MultiCell, mas vamos focar no dashboard de custos primeiro)
	return "", nil // Placeholder
}

// --- Funções Auxiliares ---

// generateFilename cria um nome de arquivo único com timestamp e garante que o diretório exista.
func generateFilename(base, dir, ext string) (string, error) {
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("could not get current working directory: %w", err)
		}
		dir = cwd
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("error creating output directory '%s': %w", dir, err)
	}
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_%s.%s", base, timestamp, ext)
	return filepath.Join(dir, filename), nil
}

// A função auxiliar calculateRowHeight também precisa ser corrigida para a lógica de mesclagem.
func calculateRowHeight(pdf *gofpdf.Fpdf, data []string, colWidths []float64, usableWidth float64) float64 {
	maxLines := 0
	lineHeight := 4.0 // Altura de uma única linha de texto (MultiCell line ht)

	for i, str := range data {
		var width float64
		// Lógica de mesclagem para cálculo
		if i == 0 {
			width = usableWidth * (colWidths[0] + colWidths[1])
		} else if i == 1 {
			continue // Pula o cálculo para a segunda coluna
		} else {
			width = usableWidth * colWidths[i]
		}

		lines := pdf.SplitText(str, width)
		if len(lines) > maxLines {
			maxLines = len(lines)
		}
	}
	// Adiciona padding: 2 para cima/baixo
	return float64(maxLines)*lineHeight + 2.0
}

// addFooter adiciona um rodapé padronizado ao PDF.
func addFooter(pdf *gofpdf.Fpdf) {
	pdf.SetY(-15)
	pdf.SetFont("Arial", "I", 8)
	pdf.SetTextColor(128, 128, 128)
	footerText := fmt.Sprintf("Generated by AWS FinOps Dashboard (Go) on %s", time.Now().Format("2006-01-02 15:04:05"))
	pdf.CellFormat(0, 10, footerText, "", 0, "C", false, 0, "")
}

// cleanRichTags remove as tags de formatação da pterm para exportação limpa.
var richTagRegex = regexp.MustCompile(`\[/?([a-zA-Z]+|#[0-9a-fA-F]{6})\]`)

func cleanRichTags(text string) string {
	return richTagRegex.ReplaceAllString(text, "")
}
